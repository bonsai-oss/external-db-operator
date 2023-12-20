package lifecycle

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"external-db-operator/internal/database"
	resourcesv1 "external-db-operator/internal/resources/v1"
)

const SecretPrefix = "edb-"

func (m *Manager) handleEvent(event watch.Event) error {
	databaseResourceData, convertError := resourcesv1.FromUnstructured(event.Object)
	if convertError != nil {
		return fmt.Errorf("failed to convert unstructured object: %w", convertError)
	}

	connectionInfo := m.clients.Database.GetConnectionInfo()

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

	existingSecret, getExistingSecretError := m.clients.Kubernetes.CoreV1().Secrets(databaseResourceData.Namespace).Get(context.Background(), secretData.Name, metav1.GetOptions{})
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
		databaseActionError = m.clients.Database.Apply(database.CreateOptions{
			Name:     databaseResourceData.AssembleDatabaseName(),
			Password: secretData.StringData["password"],
		})

		var secretError error
		if errors.IsNotFound(getExistingSecretError) {
			slog.Info("creating secret", slog.String("name", secretData.Name), slog.String("namespace", databaseResourceData.Namespace))
			_, secretError = m.clients.Kubernetes.CoreV1().Secrets(databaseResourceData.Namespace).Create(context.Background(), secretData, metav1.CreateOptions{})
		} else {
			slog.Info("updating secret", slog.String("name", secretData.Name), slog.String("namespace", databaseResourceData.Namespace))
			_, secretError = m.clients.Kubernetes.CoreV1().Secrets(databaseResourceData.Namespace).Update(context.Background(), secretData, metav1.UpdateOptions{})
		}
		if secretError != nil {
			panic(secretError.Error())
		}
	case watch.Deleted:
		databaseActionError = m.clients.Database.Destroy(database.DestroyOptions{
			Name: databaseResourceData.AssembleDatabaseName(),
		})

		slog.Info("deleting secret", slog.String("name", secretData.Name), slog.String("namespace", databaseResourceData.Namespace))
		secretDeleteError := m.clients.Kubernetes.CoreV1().Secrets(databaseResourceData.Namespace).Delete(context.Background(), secretData.Name, metav1.DeleteOptions{})
		if secretDeleteError != nil && !errors.IsNotFound(secretDeleteError) {
			panic(secretDeleteError.Error())
		}
	}
	if databaseActionError != nil {
		panic(databaseActionError.Error())
	}

	return nil
}
