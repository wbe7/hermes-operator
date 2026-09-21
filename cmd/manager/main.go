package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	v1 "github.com/wbe7/hermes-operator/api/v1alpha1"
	"github.com/wbe7/hermes-operator/internal/controller"
	"github.com/wbe7/hermes-operator/internal/settings"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	ctrl.SetLogger(logr.FromSlogHandler(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	var networkPath, metricsAddr, healthAddr, leaderNamespace string
	var leaderElection bool
	flag.StringVar(&networkPath, "network-config", "/etc/hermes-operator/network-policy.yaml", "Path to the networkPolicy object (without an enclosing key)")
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Metrics listener")
	flag.StringVar(&healthAddr, "health-probe-bind-address", ":8081", "Health listener")
	flag.StringVar(&leaderNamespace, "leader-election-namespace", os.Getenv("POD_NAMESPACE"), "Controller namespace for the leader election Lease")
	flag.BoolVar(&leaderElection, "leader-elect", true, "Use a Lease to elect one active controller")
	flag.Parse()
	raw, err := os.ReadFile(networkPath)
	if err != nil {
		return fmt.Errorf("cannot read network configuration")
	}
	network, err := settings.DecodeNetworkConfig(raw)
	if err != nil {
		return fmt.Errorf("invalid installation network configuration: %w", err)
	}
	if leaderElection && leaderNamespace == "" {
		return fmt.Errorf("leader-election-namespace or POD_NAMESPACE is required")
	}
	scheme := runtime.NewScheme()
	if err = clientgoscheme.AddToScheme(scheme); err != nil {
		return err
	}
	if err = v1.AddToScheme(scheme); err != nil {
		return err
	}
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("cannot load Kubernetes client configuration")
	}
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Cache: cache.Options{ReaderFailOnMissingInformer: true}, Metrics: metricsserver.Options{BindAddress: metricsAddr}, HealthProbeBindAddress: healthAddr, LeaderElection: leaderElection, LeaderElectionID: "hermes-operator.hermes.wbe7.github.io", LeaderElectionNamespace: leaderNamespace, LeaderElectionReleaseOnCancel: true})
	if err != nil {
		return fmt.Errorf("cannot initialize controller manager")
	}
	reconciler := &controller.HermesReconciler{Network: network}
	if err = reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("cannot configure Hermes controller: %w", err)
	}
	if err = mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err = mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}
	return mgr.Start(ctrl.SetupSignalHandler())
}
