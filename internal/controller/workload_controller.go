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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	registryv1alpha1 "github.com/bcfmtolgahan/lookout-operator/api/v1alpha1"
	"github.com/bcfmtolgahan/lookout-operator/pkg/annotations"
	"github.com/bcfmtolgahan/lookout-operator/pkg/cache"
	"github.com/bcfmtolgahan/lookout-operator/pkg/circuitbreaker"
	"github.com/bcfmtolgahan/lookout-operator/pkg/metrics"
	"github.com/bcfmtolgahan/lookout-operator/pkg/policy"
	"github.com/bcfmtolgahan/lookout-operator/pkg/ratelimit"
	"github.com/bcfmtolgahan/lookout-operator/pkg/registry"
	"github.com/bcfmtolgahan/lookout-operator/pkg/workload"
)

// WorkloadReconciler reconciles Deployment, DaemonSet, and StatefulSet objects
type WorkloadReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	Recorder            record.EventRecorder
	RegistryFactory     *registry.Factory
	ImageCache          *cache.ImageCache
	PolicyEngine        *policy.Engine
	RateLimiters        *ratelimit.Registry
	CircuitBreakers     *circuitbreaker.Registry
	Patcher             *workload.Patcher
	DefaultScanInterval time.Duration
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// ReconcileDeployment handles Deployment reconciliation
func (r *WorkloadReconciler) ReconcileDeployment(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("kind", "Deployment")

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileWorkload(ctx, logger, deployment, deployment.Annotations, &deployment.Spec.Template.Spec)
}

// ReconcileDaemonSet handles DaemonSet reconciliation
func (r *WorkloadReconciler) ReconcileDaemonSet(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("kind", "DaemonSet")

	daemonset := &appsv1.DaemonSet{}
	if err := r.Get(ctx, req.NamespacedName, daemonset); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileWorkload(ctx, logger, daemonset, daemonset.Annotations, &daemonset.Spec.Template.Spec)
}

// ReconcileStatefulSet handles StatefulSet reconciliation
func (r *WorkloadReconciler) ReconcileStatefulSet(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("kind", "StatefulSet")

	statefulset := &appsv1.StatefulSet{}
	if err := r.Get(ctx, req.NamespacedName, statefulset); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileWorkload(ctx, logger, statefulset, statefulset.Annotations, &statefulset.Spec.Template.Spec)
}

// reconcileWorkload handles the common reconciliation logic
func (r *WorkloadReconciler) reconcileWorkload(ctx context.Context, logger logr.Logger, obj client.Object, ann map[string]string, podSpec *corev1.PodSpec) (ctrl.Result, error) {
	startTime := time.Now()
	defer func() {
		metrics.WorkloadReconcileDuration.WithLabelValues(obj.GetNamespace(), obj.GetName()).Observe(time.Since(startTime).Seconds())
	}()

	// Parse annotations
	cfg, err := annotations.Parse(ann)
	if err != nil {
		logger.Error(err, "Failed to parse annotations")
		return ctrl.Result{}, err
	}

	// Check if enabled
	if !cfg.Enabled {
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling workload", "namespace", obj.GetNamespace(), "name", obj.GetName())

	// Check if suspended
	if cfg.Suspend {
		logger.Info("Workload is suspended, skipping")
		return ctrl.Result{RequeueAfter: cfg.Default.Interval}, nil
	}

	// Check maintenance window
	if cfg.MaintenanceWindow != "" {
		if !isInMaintenanceWindow(cfg.MaintenanceWindow, cfg.MaintenanceDays, cfg.MaintenanceTimezone) {
			logger.Info("Outside maintenance window, skipping")
			nextWindow := calculateNextMaintenanceWindow(cfg.MaintenanceWindow, cfg.MaintenanceDays, cfg.MaintenanceTimezone)
			return ctrl.Result{RequeueAfter: time.Until(nextWindow)}, nil
		}
	}

	// Get registry
	registryNamespace := cfg.RegistryNamespace
	if registryNamespace == "" {
		registryNamespace = obj.GetNamespace()
	}

	imageRegistry := &registryv1alpha1.ImageRegistry{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: registryNamespace,
		Name:      cfg.RegistryName,
	}, imageRegistry); err != nil {
		logger.Error(err, "Failed to get ImageRegistry", "registry", cfg.Registry)
		r.Recorder.Eventf(obj, corev1.EventTypeWarning, "RegistryNotFound", "ImageRegistry %s not found", cfg.Registry)
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Get registry client
	registryClient, err := r.RegistryFactory.GetOrCreate(ctx, imageRegistry)
	if err != nil {
		logger.Error(err, "Failed to get registry client")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Check circuit breaker
	breaker := r.CircuitBreakers.Get(imageRegistry.Name)
	if !breaker.Allow() {
		logger.Info("Circuit breaker is open, skipping")
		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Process containers
	var updates []workload.ImageUpdate

	for _, container := range podSpec.Containers {
		containerCfg := cfg.GetContainerConfig(container.Name, false)
		if !containerCfg.Enabled {
			continue
		}

		update, err := r.processContainer(ctx, logger, imageRegistry.Name, registryClient, container, containerCfg, false)
		if err != nil {
			logger.Error(err, "Failed to process container", "container", container.Name)
			continue
		}

		if update != nil {
			updates = append(updates, *update)
		}
	}

	// Process init containers
	for _, container := range podSpec.InitContainers {
		containerCfg := cfg.GetContainerConfig(container.Name, true)
		if !containerCfg.Enabled {
			continue
		}

		update, err := r.processContainer(ctx, logger, imageRegistry.Name, registryClient, container, containerCfg, true)
		if err != nil {
			logger.Error(err, "Failed to process init container", "container", container.Name)
			continue
		}

		if update != nil {
			updates = append(updates, *update)
		}
	}

	// Apply updates if any
	if len(updates) > 0 {
		// Check approval if required
		if cfg.ApprovalRequired {
			// Check if approved
			approvedImage := ""
			if len(updates) == 1 {
				approvedImage = updates[0].NewTag
			}

			if !cfg.IsApproved(approvedImage) {
				logger.Info("Update pending approval", "updates", len(updates))
				r.Recorder.Eventf(obj, corev1.EventTypeNormal, "ApprovalRequired",
					"Update to %s pending approval", updates[0].NewTag)

				// Set available update annotation
				if err := r.Patcher.SetAvailableUpdate(ctx, obj, updates[0].NewTag); err != nil {
					logger.Error(err, "Failed to set available update")
				}

				return ctrl.Result{RequeueAfter: cfg.Default.Interval}, nil
			}
		}

		// Apply updates based on workload type
		var result *workload.UpdateResult
		switch w := obj.(type) {
		case *appsv1.Deployment:
			result, err = r.Patcher.PatchDeployment(ctx, w, updates)
		case *appsv1.DaemonSet:
			result, err = r.Patcher.PatchDaemonSet(ctx, w, updates)
		case *appsv1.StatefulSet:
			result, err = r.Patcher.PatchStatefulSet(ctx, w, updates)
		}

		if err != nil {
			logger.Error(err, "Failed to apply updates")
			r.Recorder.Eventf(obj, corev1.EventTypeWarning, "UpdateFailed", "Failed to update: %v", err)
			metrics.RecordImageUpdate(obj.GetNamespace(), obj.GetName(), "", "failed")
			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}

		// Record success
		for _, update := range result.Updates {
			logger.Info("Updated container image",
				"container", update.ContainerName,
				"oldImage", update.OldImage,
				"newImage", update.NewImage)

			r.Recorder.Eventf(obj, corev1.EventTypeNormal, "ImageUpdated",
				"Updated %s from %s to %s", update.ContainerName, update.OldImage, update.NewImage)

			metrics.RecordImageUpdate(obj.GetNamespace(), obj.GetName(), update.ContainerName, "success")
		}
	}

	// Clear any previous error
	if err := r.Patcher.ClearError(ctx, obj); err != nil {
		logger.Error(err, "Failed to clear error annotation")
	}

	return ctrl.Result{RequeueAfter: cfg.Default.Interval}, nil
}

// processContainer processes a single container and returns an update if needed
func (r *WorkloadReconciler) processContainer(ctx context.Context, logger logr.Logger, registryName string, registryClient registry.Registry, container corev1.Container, cfg *annotations.ContainerConfig, isInit bool) (*workload.ImageUpdate, error) {
	// Parse current image
	currentRegistry, currentRepo, currentTag, currentDigest := workload.ParseImageReference(container.Image)

	// Get repository from config or image
	repository := cfg.Repository
	if repository == "" {
		repository = currentRepo
	}

	// Check rate limiter
	limiter := r.RateLimiters.Get(registryName)
	if !limiter.Allow() {
		if err := limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait cancelled: %w", err)
		}
	}

	// Get cached images or fetch from registry
	cached, found := r.ImageCache.Get(registryName, repository)
	if !found || cached.IsExpired() {
		// Fetch from registry
		startTime := time.Now()
		images, err := registryClient.ListTags(ctx, repository, registry.DefaultListOptions())
		duration := time.Since(startTime).Seconds()
		metrics.RecordRegistryScan(registryName, repository, duration, err)

		if err != nil {
			return nil, fmt.Errorf("failed to list tags: %w", err)
		}

		r.ImageCache.Set(registryName, repository, images, cfg.Interval)
		cached, _ = r.ImageCache.Get(registryName, repository)
	}

	// Create policy
	policyCfg := &policy.Config{
		PolicyType:  cfg.Policy,
		SemverRange: cfg.SemverRange,
		TagPattern:  cfg.TagPattern,
		ExcludeTags: cfg.ExcludeTags,
		IncludeTags: cfg.IncludeTags,
	}

	p, err := r.PolicyEngine.Create(policyCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create policy: %w", err)
	}

	// Select best image
	selectedImage := p.Select(cached.Images)
	if selectedImage == nil {
		logger.Info("No matching image found", "container", container.Name, "repository", repository)
		return nil, nil
	}

	// Check if update is needed
	needsUpdate := false

	switch cfg.Strategy {
	case annotations.StrategyTag:
		needsUpdate = currentTag != selectedImage.Tag
	case annotations.StrategyDigest:
		needsUpdate = currentDigest != selectedImage.Digest
	case annotations.StrategyTagAndDigest:
		fallthrough
	default:
		needsUpdate = currentTag != selectedImage.Tag || (currentDigest != "" && currentDigest != selectedImage.Digest)
	}

	if !needsUpdate {
		return nil, nil
	}

	// Build new image reference
	newImage := workload.BuildImageReference(currentRegistry, repository, selectedImage.Tag, selectedImage.Digest, cfg.Strategy)

	return &workload.ImageUpdate{
		ContainerName:   container.Name,
		IsInitContainer: isInit,
		OldImage:        container.Image,
		NewImage:        newImage,
		NewTag:          selectedImage.Tag,
		NewDigest:       selectedImage.Digest,
	}, nil
}

// hasLookoutAnnotation checks if the workload has Lookout annotations
func hasLookoutAnnotation(obj client.Object) bool {
	ann := obj.GetAnnotations()
	if ann == nil {
		return false
	}
	v, ok := ann[annotations.Enabled]
	return ok && v == "true"
}

// SetupWithManager sets up the controller with the Manager
func (r *WorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Predicate to only watch workloads with lookout annotation
	lookoutPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return hasLookoutAnnotation(obj)
	})

	// Setup Deployment controller
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("deployment-controller").
		For(&appsv1.Deployment{}, builder.WithPredicates(lookoutPredicate)).
		Watches(
			&registryv1alpha1.ImageRegistry{},
			handler.EnqueueRequestsFromMapFunc(r.findDeploymentsForRegistry),
		).
		Complete(reconcile.Func(r.ReconcileDeployment)); err != nil {
		return err
	}

	// Setup DaemonSet controller
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("daemonset-controller").
		For(&appsv1.DaemonSet{}, builder.WithPredicates(lookoutPredicate)).
		Watches(
			&registryv1alpha1.ImageRegistry{},
			handler.EnqueueRequestsFromMapFunc(r.findDaemonSetsForRegistry),
		).
		Complete(reconcile.Func(r.ReconcileDaemonSet)); err != nil {
		return err
	}

	// Setup StatefulSet controller
	if err := ctrl.NewControllerManagedBy(mgr).
		Named("statefulset-controller").
		For(&appsv1.StatefulSet{}, builder.WithPredicates(lookoutPredicate)).
		Watches(
			&registryv1alpha1.ImageRegistry{},
			handler.EnqueueRequestsFromMapFunc(r.findStatefulSetsForRegistry),
		).
		Complete(reconcile.Func(r.ReconcileStatefulSet)); err != nil {
		return err
	}

	return nil
}

// findDeploymentsForRegistry finds all Deployments that use a specific registry
func (r *WorkloadReconciler) findDeploymentsForRegistry(ctx context.Context, obj client.Object) []reconcile.Request {
	registry := obj.(*registryv1alpha1.ImageRegistry)
	var requests []reconcile.Request

	deployments := &appsv1.DeploymentList{}
	if err := r.List(ctx, deployments); err != nil {
		return requests
	}

	for _, d := range deployments.Items {
		if hasLookoutAnnotation(&d) {
			cfg, _ := annotations.Parse(d.Annotations)
			if cfg != nil && (cfg.RegistryName == registry.Name || cfg.Registry == registry.Name) {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: d.Namespace,
						Name:      d.Name,
					},
				})
			}
		}
	}

	return requests
}

// findDaemonSetsForRegistry finds all DaemonSets that use a specific registry
func (r *WorkloadReconciler) findDaemonSetsForRegistry(ctx context.Context, obj client.Object) []reconcile.Request {
	registry := obj.(*registryv1alpha1.ImageRegistry)
	var requests []reconcile.Request

	daemonsets := &appsv1.DaemonSetList{}
	if err := r.List(ctx, daemonsets); err != nil {
		return requests
	}

	for _, d := range daemonsets.Items {
		if hasLookoutAnnotation(&d) {
			cfg, _ := annotations.Parse(d.Annotations)
			if cfg != nil && (cfg.RegistryName == registry.Name || cfg.Registry == registry.Name) {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: d.Namespace,
						Name:      d.Name,
					},
				})
			}
		}
	}

	return requests
}

// findStatefulSetsForRegistry finds all StatefulSets that use a specific registry
func (r *WorkloadReconciler) findStatefulSetsForRegistry(ctx context.Context, obj client.Object) []reconcile.Request {
	registry := obj.(*registryv1alpha1.ImageRegistry)
	var requests []reconcile.Request

	statefulsets := &appsv1.StatefulSetList{}
	if err := r.List(ctx, statefulsets); err != nil {
		return requests
	}

	for _, s := range statefulsets.Items {
		if hasLookoutAnnotation(&s) {
			cfg, _ := annotations.Parse(s.Annotations)
			if cfg != nil && (cfg.RegistryName == registry.Name || cfg.Registry == registry.Name) {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: s.Namespace,
						Name:      s.Name,
					},
				})
			}
		}
	}

	return requests
}

// isInMaintenanceWindow checks if current time is in maintenance window
func isInMaintenanceWindow(window string, days []string, tz string) bool {
	// Parse timezone
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}

	now := time.Now().In(loc)

	// Check day
	if len(days) > 0 && days[0] != "*" {
		currentDay := now.Weekday().String()[:3]
		found := false
		for _, day := range days {
			if day == currentDay {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Parse window (format: "HH:MM-HH:MM")
	var startHour, startMin, endHour, endMin int
	if _, err := fmt.Sscanf(window, "%d:%d-%d:%d", &startHour, &startMin, &endHour, &endMin); err != nil {
		return true // If can't parse, allow
	}

	startMinutes := startHour*60 + startMin
	endMinutes := endHour*60 + endMin
	currentMinutes := now.Hour()*60 + now.Minute()

	if endMinutes < startMinutes {
		// Window crosses midnight
		return currentMinutes >= startMinutes || currentMinutes < endMinutes
	}

	return currentMinutes >= startMinutes && currentMinutes < endMinutes
}

// calculateNextMaintenanceWindow calculates the next maintenance window
func calculateNextMaintenanceWindow(window string, days []string, tz string) time.Time {
	loc := time.UTC
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}

	now := time.Now().In(loc)

	// Parse start time
	var startHour, startMin int
	if _, err := fmt.Sscanf(window, "%d:%d", &startHour, &startMin); err != nil {
		return now.Add(1 * time.Hour)
	}

	// Find next valid day
	for i := 0; i < 8; i++ {
		candidate := now.AddDate(0, 0, i)
		candidateDay := candidate.Weekday().String()[:3]

		dayValid := len(days) == 0 || days[0] == "*"
		for _, day := range days {
			if day == candidateDay {
				dayValid = true
				break
			}
		}

		if dayValid {
			nextWindow := time.Date(candidate.Year(), candidate.Month(), candidate.Day(),
				startHour, startMin, 0, 0, loc)
			if nextWindow.After(now) {
				return nextWindow
			}
		}
	}

	return now.Add(24 * time.Hour)
}
