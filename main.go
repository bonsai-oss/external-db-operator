package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kingpin/v2"
	"github.com/hellofresh/health-go/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"external-db-operator/internal/database"
	_ "external-db-operator/internal/database/mysql"
	_ "external-db-operator/internal/database/postgres"
	"external-db-operator/internal/lifecycle"
)

func mustParseSettings() Settings {
	var settings Settings
	app := kingpin.New(programName, "A Kubernetes operator for managing external databases.")
	app.HelpFlag.Short('h')

	app.Flag("database-provider", "The database provider to use.").
		Short('p').
		Envar("DATABASE_PROVIDER").
		Default("postgres").
		EnumVar(&settings.DatabaseProvider, database.ListProviders()...)

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

	app.Flag("secret-prefix", "The prefix to use for the secret names.").
		Short('s').
		Envar("SECRET_PREFIX").
		Default("edb").
		StringVar(&settings.SecretPrefix)

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

func (app *Application) startSelfService(ctx context.Context) {
	healthCheck, _ := health.New(
		health.WithComponent(health.Component{Name: programName}),
		health.WithChecks(health.Config{
			Name:  "database",
			Check: app.Clients.Database.HealthCheck,
		}))

	healthCheckHandler := http.NewServeMux()
	healthCheckHandler.Handle("/status", healthCheck.Handler())
	healthCheckHandler.Handle("/metrics", promhttp.Handler())
	server := &http.Server{Addr: ":8080", Handler: healthCheckHandler}

	go func() {
		if err := server.ListenAndServe(); err != nil {
			ctx.Done()
		}
	}()
	defer server.Shutdown(ctx)

	<-ctx.Done()
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
	SecretPrefix     string
}

type Application struct {
	Clients lifecycle.Clients
}

const (
	programName = "external-db-operator"
	// resourceLabelDifferentiator is used to differentiate between different instances of the operator. This needs to be set in the resource definition of the database objects.
	resourceLabelDifferentiator = "bonsai-oss.org/" + programName
	// maxEmptyEventsCount describes the maximum number of empty events to receive before terminating the operator.
	maxEmptyEventsCount = 10
)

func main() {
	rootContext, cancelRootContext := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	defer cancelRootContext()

	settings := mustParseSettings()
	application := &Application{}
	application.mustConfigureDatabaseProvider(settings)
	defer application.Clients.Database.Close()
	application.mustConfigureKubernetesClient()
	go application.startSelfService(rootContext)

	slog.Info("checking database connection")
	if healthCheckError := application.Clients.Database.HealthCheck(rootContext); healthCheckError != nil {
		slog.Error("failed to connect to database", slog.String("error", healthCheckError.Error()))
		return
	}

	lifecycleManager := lifecycle.NewManager(application.Clients, settings.SecretPrefix)
	go lifecycleManager.Run(rootContext)

	labelSelectorValue := fmt.Sprintf("%s-%s", settings.DatabaseProvider, settings.InstanceName)
	slog.Info("watching resources with", slog.String(resourceLabelDifferentiator, labelSelectorValue))

	for {
		select {
		case <-rootContext.Done():
			return
		default:
			watcher, watchInitError := application.Clients.KubernetesDynamic.Resource(schema.GroupVersionResource(metav1.GroupVersionResource{
				Group:    "bonsai-oss.org",
				Version:  "v1",
				Resource: "databases",
			})).Namespace("").Watch(rootContext, metav1.ListOptions{
				Watch:         true,
				LabelSelector: fmt.Sprintf("%s=%s", resourceLabelDifferentiator, labelSelectorValue),
			})
			if watchInitError != nil {
				slog.Error("failed to initialize watch", slog.String("error", watchInitError.Error()))
				os.Exit(1)
			}

			eventProcessorError := func() error {
				for {
					select {
					case <-rootContext.Done():
						return fmt.Errorf("received termination signal, shutting down")
					case event, ok := <-watcher.ResultChan():
						if !ok {
							return fmt.Errorf("watcher closed unexpectedly")
						}
						lifecycleManager.Events <- event
					}
				}
			}()
			slog.Error("failed to process events", slog.String("error", eventProcessorError.Error()))
		}
	}
}
