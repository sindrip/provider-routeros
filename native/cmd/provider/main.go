package main

import (
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	providerv1alpha1 "github.com/sindrip/provider-routeros/native/api/provider/v1alpha1"
	v1alpha1 "github.com/sindrip/provider-routeros/native/api/v1alpha1"
	"github.com/sindrip/provider-routeros/native/internal/controller/firewallfilter"
	"github.com/sindrip/provider-routeros/native/internal/controller/providerconfig"
)

func main() {
	var metricsAddress string
	var probeAddress string
	var leaderElection bool
	flag.StringVar(&metricsAddress, "metrics-bind-address", ":8081", "Address for the metrics endpoint.")
	flag.StringVar(&probeAddress, "health-probe-bind-address", ":8082", "Address for health probes.")
	flag.BoolVar(&leaderElection, "leader-elect", false, "Use leader election.")
	logging := zap.Options{Development: false}
	logging.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logging)))

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(addProviderConfigTypes(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: metricsAddress},
		HealthProbeBindAddress: probeAddress,
		LeaderElection:         leaderElection,
		LeaderElectionID:       "native.provider-routeros.m.sindrip.io",
	})
	if err != nil {
		ctrl.Log.Error(err, "create manager")
		os.Exit(1)
	}
	if err := (&firewallfilter.Reconciler{Client: mgr.GetClient()}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "register firewall filter controller")
		os.Exit(1)
	}
	if err := providerconfig.SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "register ProviderConfig controller")
		os.Exit(1)
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "register health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		ctrl.Log.Error(err, "register readiness check")
		os.Exit(1)
	}
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "run manager")
		os.Exit(1)
	}
}

// addProviderConfigTypes deliberately omits ClusterProviderConfig. The native
// API only resolves namespaced ProviderConfig objects.
func addProviderConfigTypes(scheme *runtime.Scheme) error {
	return providerv1alpha1.AddToScheme(scheme)
}
