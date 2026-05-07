package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	minecraftv1alpha1 "github.com/luisito666/mc-proxy-operator/api/v1alpha1"
	"github.com/luisito666/mc-proxy-operator/internal/controller"
	"github.com/luisito666/mc-proxy-operator/internal/proxy"
	"github.com/luisito666/mc-proxy-operator/internal/proxy/bedrock"
	"github.com/luisito666/mc-proxy-operator/internal/proxy/java"
	"github.com/luisito666/mc-proxy-operator/internal/proxy/portmanager"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(minecraftv1alpha1.AddToScheme(scheme))
}

func main() {
	opts := zap.Options{Development: true}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	metricsAddr := envOrDefault("METRICS_ADDR", ":8080")
	probeAddr := envOrDefault("HEALTH_PROBE_ADDR", ":8081")
	javaAddr := envOrDefault("JAVA_LISTEN_ADDR", ":25565")

	routeTable := proxy.NewRouteTable()
	portMgr := portmanager.NewPortManager(
		portmanager.DefaultMinPort,
		portmanager.DefaultMaxPort,
	)

	registry := proxy.NewHandlerRegistry()

	javaHandler := java.NewJavaProtocolHandler(javaAddr)
	registry.Register(javaHandler)

	bedrockHandler := bedrock.NewBedrockProtocolHandler()
	registry.Register(bedrockHandler)

	setupLog.Info("protocol handlers registrados",
		"editions", registry.ListEditions(),
	)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         false,
	})
	if err != nil {
		setupLog.Error(err, "error creando manager")
		os.Exit(1)
	}

	if err := (&controller.MinecraftProxyReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		RouteTable:      routeTable,
		HandlerRegistry: registry,
		PortManager:     portMgr,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "error configurando controller")
		os.Exit(1)
	}

	mgr.AddHealthzCheck("healthz", healthz.Ping)
	mgr.AddReadyzCheck("readyz", healthz.Ping)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := registry.StartAll(ctx, routeTable); err != nil {
		setupLog.Error(err, "error iniciando handlers")
		os.Exit(1)
	}

	setupLog.Info("iniciando manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "error ejecutando manager")
		os.Exit(1)
	}

	registry.StopAll()
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
