package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	EventProcessing = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "external_db_operator",
		Name:      "event_processing_duration_seconds",
	}, []string{"event_type"})
)
