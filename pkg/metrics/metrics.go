/*
Copyright 2026.

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

const (
	namespace = "lookout"
)

var (
	// Registry metrics
	RegistryScanDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "registry_scan_duration_seconds",
			Help:      "Time taken to scan a registry repository",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"registry", "repository"},
	)

	RegistryScansTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "registry_scans_total",
			Help:      "Total number of registry scans",
		},
		[]string{"registry", "status"}, // success, error
	)

	RegistryScanErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "registry_scan_errors_total",
			Help:      "Total number of registry scan errors",
		},
		[]string{"registry", "error_type"}, // auth, network, rate_limit, timeout
	)

	RegistryCircuitState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "registry_circuit_state",
			Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		},
		[]string{"registry"},
	)

	RegistryRateLimitRemaining = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "registry_rate_limit_remaining",
			Help:      "Remaining rate limit tokens",
		},
		[]string{"registry"},
	)

	// Workload metrics
	WorkloadsWatched = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "workloads_watched",
			Help:      "Number of workloads being watched",
		},
		[]string{"namespace", "kind"},
	)

	WorkloadReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "workload_reconcile_duration_seconds",
			Help:      "Time taken to reconcile a workload",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5},
		},
		[]string{"namespace", "name"},
	)

	// Update metrics
	ImageUpdatesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "image_updates_total",
			Help:      "Total number of image updates applied",
		},
		[]string{"namespace", "workload", "container", "status"}, // success, failed, skipped
	)

	ImageUpdateDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "image_update_duration_seconds",
			Help:      "Time taken to apply an image update",
			Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10},
		},
		[]string{"namespace", "workload"},
	)

	// Rollback metrics
	RollbacksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rollbacks_total",
			Help:      "Total number of rollbacks performed",
		},
		[]string{"namespace", "workload", "trigger"}, // auto, manual
	)

	// Cache metrics
	CacheRepositories = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cache_repositories",
			Help:      "Number of repositories in cache",
		},
		[]string{"registry"},
	)

	CacheTags = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cache_tags",
			Help:      "Number of tags in cache",
		},
		[]string{"registry"},
	)

	CacheHits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_hits_total",
			Help:      "Total cache hits",
		},
	)

	CacheMisses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_misses_total",
			Help:      "Total cache misses",
		},
	)

	// Business metrics
	PendingApprovals = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "pending_approvals",
			Help:      "Number of updates pending approval",
		},
		[]string{"namespace"},
	)

	SuspendedWorkloads = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "suspended_workloads",
			Help:      "Number of workloads with updates suspended",
		},
		[]string{"namespace"},
	)

	AvailableUpdates = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "available_updates",
			Help:      "Number of available but not applied updates",
		},
		[]string{"namespace", "workload", "reason"}, // suspended, outside_window, pending_approval
	)

	ImageAge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "image_age_seconds",
			Help:      "Age of the currently deployed image",
		},
		[]string{"namespace", "workload", "container"},
	)
)

func init() {
	// Register all metrics
	metrics.Registry.MustRegister(
		// Registry metrics
		RegistryScanDuration,
		RegistryScansTotal,
		RegistryScanErrors,
		RegistryCircuitState,
		RegistryRateLimitRemaining,

		// Workload metrics
		WorkloadsWatched,
		WorkloadReconcileDuration,

		// Update metrics
		ImageUpdatesTotal,
		ImageUpdateDuration,

		// Rollback metrics
		RollbacksTotal,

		// Cache metrics
		CacheRepositories,
		CacheTags,
		CacheHits,
		CacheMisses,

		// Business metrics
		PendingApprovals,
		SuspendedWorkloads,
		AvailableUpdates,
		ImageAge,
	)
}

// RecordRegistryScan records a registry scan
func RecordRegistryScan(registry, repository string, duration float64, err error) {
	RegistryScanDuration.WithLabelValues(registry, repository).Observe(duration)

	status := "success"
	if err != nil {
		status = "error"
	}
	RegistryScansTotal.WithLabelValues(registry, status).Inc()
}

// RecordRegistryError records a registry error
func RecordRegistryError(registry, errorType string) {
	RegistryScanErrors.WithLabelValues(registry, errorType).Inc()
}

// RecordImageUpdate records an image update
func RecordImageUpdate(namespace, workload, container, status string) {
	ImageUpdatesTotal.WithLabelValues(namespace, workload, container, status).Inc()
}

// RecordRollback records a rollback
func RecordRollback(namespace, workload, trigger string) {
	RollbacksTotal.WithLabelValues(namespace, workload, trigger).Inc()
}

// SetCircuitState sets the circuit breaker state
// 0 = closed, 1 = open, 2 = half-open
func SetCircuitState(registry string, state int) {
	RegistryCircuitState.WithLabelValues(registry).Set(float64(state))
}

// UpdateCacheMetrics updates cache metrics
func UpdateCacheMetrics(registry string, repoCount, tagCount int) {
	CacheRepositories.WithLabelValues(registry).Set(float64(repoCount))
	CacheTags.WithLabelValues(registry).Set(float64(tagCount))
}
