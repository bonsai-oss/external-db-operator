package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/alecthomas/kingpin/v2"
	"github.com/hellofresh/health-go/v5"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"external-db-operator/internal/database"
	_ "external-db-operator/internal/database/postgres"
	"external-db-operator/internal/lifecycle"
)

func mustParseSettings() Settings {
	var settings Settings
	app := kingpin.New(ProgramName, "A Kubernetes operator for managing external databases.")
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

	app.Flag("instance-name", "The name of the instance.").
		Short('i').
		Envar("INSTANCE_NAME").
		Default("default").
		StringVar(&settings.InstanceName)

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
	InstanceName     string
}

type Application struct {
	Clients lifecycle.Clients
}

const (
	ProgramName                 = "external-db-operator"
	ResourceLabelDifferentiator = "fsrv.cloud/" + ProgramName
)

func main() {
	settings := mustParseSettings()
	application := &Application{}
	application.mustConfigureDatabaseProvider(settings)
	defer application.Clients.Database.Close()
	application.mustConfigureKubernetesClient()

	healthCheck, _ := health.New(health.WithComponent(health.Component{
		Name: ProgramName,
	}), health.WithSystemInfo(), health.WithChecks(health.Config{
		Name:  "database:ping",
		Check: application.Clients.Database.HealthCheck,
	}))

	go func() {
		healthCheckHandler := http.NewServeMux()
		healthCheckHandler.HandleFunc("/status", healthCheck.HandlerFunc)
		http.ListenAndServe(":8080", healthCheckHandler)
	}()

	labelSelectorValue := fmt.Sprintf("%s-%s", settings.DatabaseProvider, settings.InstanceName)
	slog.Info("watching resources with", slog.String(ResourceLabelDifferentiator, labelSelectorValue))

	watcher, watchInitError := application.Clients.KubernetesDynamic.Resource(schema.GroupVersionResource(metav1.GroupVersionResource{
		Group:    "fsrv.cloud",
		Version:  "v1",
		Resource: "databases",
	})).Namespace("").Watch(context.Background(), metav1.ListOptions{
		Watch:         true,
		LabelSelector: fmt.Sprintf("%s=%s", ResourceLabelDifferentiator, labelSelectorValue),
	})
	if watchInitError != nil {
		slog.Error("failed to initialize watch", slog.String("error", watchInitError.Error()))
		os.Exit(1)
	}

	worker := lifecycle.NewManager(application.Clients)
	go worker.Run(context.Background())

	// empty events do occur on crd changes and trigger until the next restart of the watcher
	var emptyEventCount int

	for event := range watcher.ResultChan() {
		// terminate the operator if we receive too many empty events
		if emptyEventCount > 10 {
			slog.Error("too many empty events, exiting")
			return
		}
		if event.Type == "" {
			emptyEventCount++
			continue
		}
		worker.Events <- event
	}
}
