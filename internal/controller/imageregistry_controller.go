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

package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	registryv1alpha1 "github.com/bcfmtolgahan/lookout-operator/api/v1alpha1"
	"github.com/bcfmtolgahan/lookout-operator/pkg/circuitbreaker"
	"github.com/bcfmtolgahan/lookout-operator/pkg/metrics"
	"github.com/bcfmtolgahan/lookout-operator/pkg/registry"
)

// ImageRegistryReconciler reconciles a ImageRegistry object
type ImageRegistryReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	RegistryFactory *registry.Factory
	CircuitBreakers *circuitbreaker.Registry
}

// +kubebuilder:rbac:groups=registry.lookout.dev,resources=imageregistries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=registry.lookout.dev,resources=imageregistries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=registry.lookout.dev,resources=imageregistries/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *ImageRegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ImageRegistry
	imageRegistry := &registryv1alpha1.ImageRegistry{}
	if err := r.Get(ctx, req.NamespacedName, imageRegistry); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling ImageRegistry", "name", imageRegistry.Name, "type", imageRegistry.Spec.Type)

	// Update observed generation
	if imageRegistry.Status.ObservedGeneration != imageRegistry.Generation {
		imageRegistry.Status.ObservedGeneration = imageRegistry.Generation
	}

	// Get or create registry client
	registryClient, err := r.RegistryFactory.GetOrCreate(ctx, imageRegistry)
	if err != nil {
		logger.Error(err, "Failed to create registry client")
		r.setCondition(imageRegistry, registryv1alpha1.ConditionTypeReady, metav1.ConditionFalse,
			registryv1alpha1.ReasonConnectionFailed, err.Error())
		if err := r.Status().Update(ctx, imageRegistry); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Get circuit breaker
	breaker := r.CircuitBreakers.Get(req.String())

	// Check health with circuit breaker
	var healthErr error
	if breaker.Allow() {
		healthErr = registryClient.CheckHealth(ctx)
		if healthErr != nil {
			breaker.RecordFailure()
			logger.Error(healthErr, "Registry health check failed")
			metrics.RecordRegistryError(imageRegistry.Name, "health_check")
		} else {
			breaker.RecordSuccess()
		}
	} else {
		logger.Info("Circuit breaker is open, skipping health check")
		healthErr = circuitbreaker.ErrCircuitOpen
	}

	// Update circuit breaker status
	imageRegistry.Status.CircuitBreaker = &registryv1alpha1.CircuitBreakerStatus{
		State:    registryv1alpha1.CircuitBreakerState(breaker.State().String()),
		Failures: breaker.Failures(),
	}
	if !breaker.LastFailure().IsZero() {
		t := metav1.NewTime(breaker.LastFailure())
		imageRegistry.Status.CircuitBreaker.LastFailure = &t
	}

	// Update metrics
	metrics.SetCircuitState(imageRegistry.Name, int(breaker.State()))

	// Update conditions based on health check
	if healthErr != nil {
		if healthErr == circuitbreaker.ErrCircuitOpen {
			r.setCondition(imageRegistry, registryv1alpha1.ConditionTypeCircuitOpen, metav1.ConditionTrue,
				registryv1alpha1.ReasonCircuitOpen, "Circuit breaker is open due to repeated failures")
		}
		r.setCondition(imageRegistry, registryv1alpha1.ConditionTypeReady, metav1.ConditionFalse,
			registryv1alpha1.ReasonConnectionFailed, healthErr.Error())
	} else {
		now := metav1.Now()
		imageRegistry.Status.LastConnectedAt = &now

		r.setCondition(imageRegistry, registryv1alpha1.ConditionTypeReady, metav1.ConditionTrue,
			registryv1alpha1.ReasonConnectionSuccessful, "Successfully connected to registry")
		r.setCondition(imageRegistry, registryv1alpha1.ConditionTypeCircuitOpen, metav1.ConditionFalse,
			registryv1alpha1.ReasonCircuitClosed, "Circuit breaker is closed")
		r.setCondition(imageRegistry, registryv1alpha1.ConditionTypeAuthValid, metav1.ConditionTrue,
			registryv1alpha1.ReasonAuthValid, "Authentication is valid")

		// Update auth status
		imageRegistry.Status.Auth = &registryv1alpha1.AuthStatus{
			Valid:         true,
			LastValidated: &now,
		}
	}

	// Update status
	if err := r.Status().Update(ctx, imageRegistry); err != nil {
		logger.Error(err, "Failed to update ImageRegistry status")
		return ctrl.Result{}, err
	}

	// Requeue based on scan interval
	requeueAfter := 5 * time.Minute
	if imageRegistry.Spec.Scan != nil && imageRegistry.Spec.Scan.DefaultInterval != "" {
		if d, err := time.ParseDuration(imageRegistry.Spec.Scan.DefaultInterval); err == nil {
			requeueAfter = d
		}
	}

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// setCondition sets a condition on the ImageRegistry
func (r *ImageRegistryReconciler) setCondition(ir *registryv1alpha1.ImageRegistry, condType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ir.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&ir.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ImageRegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&registryv1alpha1.ImageRegistry{}).
		Owns(&corev1.Secret{}). // Watch secrets for auth changes
		Named("imageregistry").
		Complete(r)
}
