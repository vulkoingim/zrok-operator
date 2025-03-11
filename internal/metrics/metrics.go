/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ShareReady is the number of ZrokShare resources that are Ready.
	ShareReady = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "zrok_share_ready",
		Help: "Number of ZrokShare resources with Ready=True",
	})

	// ShareReconcileErrors counts share reconcile errors.
	ShareReconcileErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "zrok_share_reconcile_errors",
		Help: "Total number of ZrokShare reconcile errors",
	})

	// EnvironmentReady is the number of ZrokEnvironment resources that are Ready.
	EnvironmentReady = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "zrok_environment_ready",
		Help: "Number of ZrokEnvironment resources with Ready=True",
	})
)

func init() {
	metrics.Registry.MustRegister(ShareReady, ShareReconcileErrors, EnvironmentReady)
}
