package lifecycle

import (
	"context"
	"log/slog"

	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"external-db-operator/internal/database"
)

type Manager struct {
	Events  chan watch.Event
	clients Clients
}

type Clients struct {
	Kubernetes        *kubernetes.Clientset
	KubernetesDynamic *dynamic.DynamicClient
	Database          database.Provider
}

func NewManager(clients Clients) *Manager {
	if clients.Kubernetes == nil {
		panic("kubernetes client is required")
	}
	if clients.KubernetesDynamic == nil {
		panic("kubernetes dynamic client is required")
	}
	if clients.Database == nil {
		panic("database provider is required")
	}
	return &Manager{
		Events:  make(chan watch.Event),
		clients: clients,
	}
}

func (m *Manager) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-m.Events:
			handlingError := m.handleEvent(event)
			if handlingError != nil {
				slog.Error("failed to handle event", slog.String("error", handlingError.Error()))
			}
		}
	}
}
