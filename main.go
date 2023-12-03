package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/alecthomas/kingpin/v2"
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

func mustParseSettings() Settings {
	var settings Settings
	app := kingpin.New("external-db-operator", "A Kubernetes operator for managing external databases.")
	app.HelpFlag.Short('h')

	app.Flag("database-provider", "The database provider to use.").
		Short('p').
		Envar("DATABASE_PROVIDER").
		Default("postgres").
		EnumVar(&settings.DatabaseProvider, "postgres")

	app.Flag("database-dsn", "The DSN to use for the database provider.").
		Short('d').
		Envar("DATABASE_DSN").
		Default("postgres://postgres:postgres@localhost:5432/postgres").
		StringVar(&settings.DatabaseDsn)

	kingpin.MustParse(app.Parse(os.Args[1:]))

	return settings
}

func (app *Application) mustConfigureDatabaseProvider(settings Settings) {
	databaseBackend, providerError := database.Provide(settings.DatabaseProvider)
	if providerError != nil {
		slog.Error("failed to provide database backend", slog.String("error", providerError.Error()))
		os.Exit(1)
	}
	databaseInitializationError := databaseBackend.Initialize(settings.DatabaseDsn)
	if databaseInitializationError != nil {
		slog.Error("failed to initialize database backend", slog.String("error", databaseInitializationError.Error()))
		os.Exit(1)
	}

	app.Clients.Database = databaseBackend
}

func (app *Application) mustConfigureKubernetesClient() {
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

	var clientConfigurationError error
	app.Clients.KubernetesDynamic, clientConfigurationError = dynamic.NewForConfig(clientConfig)
	app.Clients.Kubernetes, clientConfigurationError = kubernetes.NewForConfig(clientConfig)
	if clientConfigurationError != nil {
		slog.Error("failed to create Kubernetes client", slog.String("error", clientConfigurationError.Error()))
		os.Exit(1)
	}
}

func init() {
	_, isDebug := os.LookupEnv("DEBUG")
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: isDebug,
		Level:     slog.LevelDebug,
	})))
}

type Settings struct {
	DatabaseProvider string
	DatabaseDsn      string
}

type Application struct {
	Clients struct {
		Kubernetes        *kubernetes.Clientset
		KubernetesDynamic *dynamic.DynamicClient
		Database          database.Provider
	}
}

func main() {
	settings := mustParseSettings()
	application := &Application{}
	application.mustConfigureDatabaseProvider(settings)
	defer application.Clients.Database.Close()
	application.mustConfigureKubernetesClient()

	connectionInfo := application.Clients.Database.GetConnectionInfo()

	const SecretPrefix = "edb-"

	watcher, watchInitError := application.Clients.KubernetesDynamic.Resource(schema.GroupVersionResource(metav1.GroupVersionResource{
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
					"host":     connectionInfo.Host,
					"port":     fmt.Sprintf("%d", connectionInfo.Port),
					"database": databaseResourceData.AssembleDatabaseName(),
				},
			}

			existingSecret, getExistingSecretError := application.Clients.Kubernetes.CoreV1().Secrets(databaseResourceData.Namespace).Get(context.Background(), secretData.Name, metav1.GetOptions{})
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
				databaseActionError = application.Clients.Database.Apply(database.CreateOptions{
					Name:     databaseResourceData.AssembleDatabaseName(),
					Password: secretData.StringData["password"],
				})

				var secretError error
				if errors.IsNotFound(getExistingSecretError) {
					slog.Info("creating secret", slog.String("name", secretData.Name), slog.String("namespace", databaseResourceData.Namespace))
					_, secretError = application.Clients.Kubernetes.CoreV1().Secrets(databaseResourceData.Namespace).Create(context.Background(), secretData, metav1.CreateOptions{})
				} else {
					slog.Info("updating secret", slog.String("name", secretData.Name), slog.String("namespace", databaseResourceData.Namespace))
					_, secretError = application.Clients.Kubernetes.CoreV1().Secrets(databaseResourceData.Namespace).Update(context.Background(), secretData, metav1.UpdateOptions{})
				}
				if secretError != nil {
					panic(secretError.Error())
				}
			case watch.Deleted:
				databaseActionError = application.Clients.Database.Destroy(database.DestroyOptions{
					Name: databaseResourceData.AssembleDatabaseName(),
				})

				slog.Info("deleting secret", slog.String("name", secretData.Name), slog.String("namespace", databaseResourceData.Namespace))
				secretDeleteError := application.Clients.Kubernetes.CoreV1().Secrets(databaseResourceData.Namespace).Delete(context.Background(), secretData.Name, metav1.DeleteOptions{})
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
