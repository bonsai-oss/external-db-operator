package lifecycle

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"external-db-operator/internal/database"
	"external-db-operator/internal/metrics"
)

type Manager struct {
	Events       chan watch.Event
	clients      Clients
	secretPrefix string
}

type Clients struct {
	Kubernetes        *kubernetes.Clientset
	KubernetesDynamic *dynamic.DynamicClient
	Database          database.Provider
}

func NewManager(clients Clients, secretPrefix string) *Manager {
	if clients.Kubernetes == nil {
		panic("kubernetes client is required")
	}
	if clients.KubernetesDynamic == nil {
		panic("kubernetes dynamic client is required")
	}
	if clients.Database == nil {
		panic("database provider is required")
	}
	if secretPrefix == "" {
		panic("secret prefix is required")
	}
	return &Manager{
		Events:       make(chan watch.Event),
		clients:      clients,
		secretPrefix: secretPrefix,
	}
}

func (m *Manager) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-m.Events:
			start := time.Now()
			handlingError := m.handleEvent(event)
			if handlingError != nil {
				slog.Error("failed to handle event", slog.String("error", handlingError.Error()))
			}
			metrics.EventProcessing.With(prometheus.Labels{
				"event_type": string(event.Type),
			}).Observe(time.Since(start).Seconds())
		}
	}
}
