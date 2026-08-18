package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	zrokv1alpha1 "github.com/vulkoingim/zrok-operator/api/v1alpha1"
	"github.com/vulkoingim/zrok-operator/internal/build"
	"github.com/vulkoingim/zrok-operator/internal/controller"
	_ "github.com/vulkoingim/zrok-operator/internal/metrics"
	"github.com/vulkoingim/zrok-operator/internal/zrokclient"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(zrokv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var printVersion bool
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true, "Serve metrics securely via HTTPS.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "", "Directory containing the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "Metrics certificate file name.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "Metrics key file name.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false, "Enable HTTP/2 for the metrics server.")
	flag.BoolVar(&printVersion, "version", false, "Print version and exit.")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	if printVersion {
		fmt.Printf("%s (%s) %s\n", build.Version, build.GitRevision, build.Date)
		os.Exit(0)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, func(c *tls.Config) {
			setupLog.Info("disabling http/2")
			c.NextProtos = []string{"http/1.1"}
		})
	}

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}
	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	var metricsCertWatcher *certwatcher.CertWatcher
	if metricsCertPath != "" {
		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize metrics certificate watcher")
			os.Exit(1)
		}
		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "f22d3959.k8s.zrok.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	zrokClients := zrokclient.NewDefaultClients(nil)

	if err = (&controller.ZrokEnvironmentReconciler{
		Client:    mgr.GetClient(),
		APIReader: mgr.GetAPIReader(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorder("zrokenvironment-controller"),
		Zrok:      zrokClients,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ZrokEnvironment")
		os.Exit(1)
	}

	if err = (&controller.ZrokShareReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorder("zrokshare-controller"),
		Zrok:     zrokClients,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ZrokShare")
		os.Exit(1)
	}

	if err = (&controller.ZrokAccessReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorder("zrokaccess-controller"),
		Zrok:     zrokClients,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ZrokAccess")
		os.Exit(1)
	}

	if err = (&controller.IngressReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorder("zrok-ingress-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Ingress")
		os.Exit(1)
	}

	if metricsCertWatcher != nil {
		if err := mgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager",
		"version", build.Version, "commit", build.GitRevision, "date", build.Date)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
