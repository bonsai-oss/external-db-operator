package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"external-db-operator/internal/database"
	_ "external-db-operator/internal/database/postgres"
	resourcesv1 "external-db-operator/internal/resources/v1"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
	})))
}

func main() {
	const SecretPrefix = "edb-"

	var clientConfig *rest.Config
	var clientConfigError error
	if os.Getenv("KUBECONFIG") == "" {
		clientConfig, clientConfigError = rest.InClusterConfig()
	} else {
		clientConfig, clientConfigError = clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	}
	if clientConfigError != nil {
		panic(clientConfigError.Error())
	}

	client, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		panic(err.Error())
	}

	clientset, clientSetCreateError := kubernetes.NewForConfig(clientConfig)
	if clientSetCreateError != nil {
		panic(clientSetCreateError.Error())
	}

	databaseBackend := database.Provide("postgres")
	databaseInitializationError := databaseBackend.Initialize("postgres://postgres:postgres@localhost:5432/postgres")
	if databaseInitializationError != nil {
		panic(databaseInitializationError.Error())
	}

	watcher, watchInitError := client.Resource(schema.GroupVersionResource(metav1.GroupVersionResource{
		Group:    "fsrv.cloud",
		Version:  "v1",
		Resource: "databases",
	})).Namespace("").Watch(context.Background(), metav1.ListOptions{
		Watch: true,
	})
	if watchInitError != nil {
		panic(watchInitError)
	}

	for {
		select {
		case event := <-watcher.ResultChan():
			if event.Type == "" {
				continue
			}

			databaseResourceData, convertError := resourcesv1.FromUnstructured(event.Object)
			if convertError != nil {
				slog.Error("failed to convert unstructured object", slog.String("error", convertError.Error()))
				continue
			}

			secretData := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: SecretPrefix + databaseResourceData.Name,
				},
				StringData: map[string]string{
					"username": databaseResourceData.AssembleDatabaseName(),
					"password": uuid.NewString(),
					"dsn":      databaseBackend.GetDSN(),
				},
			}

			existingSecret, getExistingSecretError := clientset.CoreV1().Secrets(databaseResourceData.Namespace).Get(context.Background(), secretData.Name, metav1.GetOptions{})
			if getExistingSecretError != nil && !errors.IsNotFound(getExistingSecretError) {
				panic(getExistingSecretError.Error())
			}

			if !errors.IsNotFound(getExistingSecretError) {
				// existingSecret.StringData is not populated by the Get() method
				secretData.StringData["password"] = string(existingSecret.Data["password"])
			}

			var databaseActionError error
			switch event.Type {
			case watch.Modified:
				fallthrough
			case watch.Added:
				databaseActionError = databaseBackend.Apply(database.CreateOptions{
					Name:     databaseResourceData.AssembleDatabaseName(),
					Password: secretData.StringData["password"],
				})

				var secretError error
				if errors.IsNotFound(getExistingSecretError) {
					slog.Info("creating secret", slog.String("name", secretData.Name))
					_, secretError = clientset.CoreV1().Secrets(databaseResourceData.Namespace).Create(context.Background(), secretData, metav1.CreateOptions{})
				} else {
					slog.Info("updating secret", slog.String("name", secretData.Name))
					_, secretError = clientset.CoreV1().Secrets(databaseResourceData.Namespace).Update(context.Background(), secretData, metav1.UpdateOptions{})
				}
				if secretError != nil {
					panic(secretError.Error())
				}
			case watch.Deleted:
				databaseActionError = databaseBackend.Destroy(database.DestroyOptions{
					Name: databaseResourceData.AssembleDatabaseName(),
				})

				slog.Info("deleting secret", slog.String("name", secretData.Name))
				secretDeleteError := clientset.CoreV1().Secrets(databaseResourceData.Namespace).Delete(context.Background(), secretData.Name, metav1.DeleteOptions{})
				if secretDeleteError != nil && !errors.IsNotFound(secretDeleteError) {
					panic(secretDeleteError.Error())
				}
			}
			if databaseActionError != nil {
				panic(databaseActionError.Error())
			}
		}
	}
}
