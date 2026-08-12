package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ShareReady is 1 when a ZrokShare is Ready, 0 otherwise (per object).
	ShareReady = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "zrok_share_ready",
		Help: "Whether a ZrokShare is Ready (1) or not (0)",
	}, []string{"namespace", "name"})

	// ShareReconcileErrors counts share reconcile errors.
	ShareReconcileErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "zrok_share_reconcile_errors",
		Help: "Total number of ZrokShare reconcile errors",
	})

	// EnvironmentReady is 1 when a ZrokEnvironment is Ready, 0 otherwise (per object).
	EnvironmentReady = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "zrok_environment_ready",
		Help: "Whether a ZrokEnvironment is Ready (1) or not (0)",
	}, []string{"namespace", "name"})

	// EnvironmentReconcileErrors counts environment reconcile errors.
	EnvironmentReconcileErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "zrok_environment_reconcile_errors",
		Help: "Total number of ZrokEnvironment reconcile errors",
	})

	// AccessReconcileErrors counts access reconcile errors.
	AccessReconcileErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "zrok_access_reconcile_errors",
		Help: "Total number of ZrokAccess reconcile errors",
	})
)

func init() {
	metrics.Registry.MustRegister(
		ShareReady,
		ShareReconcileErrors,
		EnvironmentReady,
		EnvironmentReconcileErrors,
		AccessReconcileErrors,
	)
}

// SetShareReady sets the per-object share ready gauge.
func SetShareReady(namespace, name string, ready bool) {
	v := 0.0
	if ready {
		v = 1
	}
	ShareReady.WithLabelValues(namespace, name).Set(v)
}

// DeleteShareReady removes the gauge series for a deleted share.
func DeleteShareReady(namespace, name string) {
	ShareReady.DeleteLabelValues(namespace, name)
}

// SetEnvironmentReady sets the per-object environment ready gauge.
func SetEnvironmentReady(namespace, name string, ready bool) {
	v := 0.0
	if ready {
		v = 1
	}
	EnvironmentReady.WithLabelValues(namespace, name).Set(v)
}

// DeleteEnvironmentReady removes the gauge series for a deleted environment.
func DeleteEnvironmentReady(namespace, name string) {
	EnvironmentReady.DeleteLabelValues(namespace, name)
}
