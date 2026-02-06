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

package workload

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/bcfmtolgahan/lookout-operator/pkg/annotations"
)

// ImageUpdate represents a pending image update
type ImageUpdate struct {
	// ContainerName is the name of the container to update
	ContainerName string

	// IsInitContainer indicates if this is an init container
	IsInitContainer bool

	// OldImage is the current image
	OldImage string

	// NewImage is the target image (full reference)
	NewImage string

	// NewTag is the new tag
	NewTag string

	// NewDigest is the new digest
	NewDigest string
}

// UpdateResult represents the result of an update
type UpdateResult struct {
	// Success indicates if the update was successful
	Success bool

	// Updates are the applied updates
	Updates []ImageUpdate

	// Error is the error if any
	Error error

	// PreviousImages maps container name to previous image
	PreviousImages map[string]string
}

// Patcher handles workload patching
type Patcher struct {
	client client.Client
}

// NewPatcher creates a new workload patcher
func NewPatcher(c client.Client) *Patcher {
	return &Patcher{client: c}
}

// PatchDeployment updates a Deployment with new images
func (p *Patcher) PatchDeployment(ctx context.Context, deployment *appsv1.Deployment, updates []ImageUpdate) (*UpdateResult, error) {
	result := &UpdateResult{
		Updates:        updates,
		PreviousImages: make(map[string]string),
	}

	// Store previous images
	for _, container := range deployment.Spec.Template.Spec.Containers {
		result.PreviousImages[container.Name] = container.Image
	}
	for _, container := range deployment.Spec.Template.Spec.InitContainers {
		result.PreviousImages["init:"+container.Name] = container.Image
	}

	// Create a copy for patching
	patch := deployment.DeepCopy()

	// Apply updates to containers
	for _, update := range updates {
		if update.IsInitContainer {
			for i := range patch.Spec.Template.Spec.InitContainers {
				if patch.Spec.Template.Spec.InitContainers[i].Name == update.ContainerName {
					patch.Spec.Template.Spec.InitContainers[i].Image = update.NewImage
					break
				}
			}
		} else {
			for i := range patch.Spec.Template.Spec.Containers {
				if patch.Spec.Template.Spec.Containers[i].Name == update.ContainerName {
					patch.Spec.Template.Spec.Containers[i].Image = update.NewImage
					break
				}
			}
		}
	}

	// Update status annotations
	if patch.Annotations == nil {
		patch.Annotations = make(map[string]string)
	}
	patch.Annotations[annotations.StatusLastUpdate] = time.Now().UTC().Format(time.RFC3339)
	patch.Annotations[annotations.PreviousImage] = formatPreviousImages(result.PreviousImages)

	// Increment update count
	count := 0
	if v, ok := deployment.Annotations[annotations.StatusUpdateCount]; ok {
		fmt.Sscanf(v, "%d", &count)
	}
	patch.Annotations[annotations.StatusUpdateCount] = fmt.Sprintf("%d", count+1)

	// Clear approval if it was set
	delete(patch.Annotations, annotations.ApprovedBy)
	delete(patch.Annotations, annotations.ApprovedImage)
	delete(patch.Annotations, annotations.StatusAvailableUpdate)

	// Patch the deployment
	if err := p.client.Patch(ctx, patch, client.MergeFrom(deployment)); err != nil {
		result.Error = fmt.Errorf("failed to patch deployment: %w", err)
		return result, result.Error
	}

	result.Success = true
	return result, nil
}

// PatchDaemonSet updates a DaemonSet with new images
func (p *Patcher) PatchDaemonSet(ctx context.Context, daemonset *appsv1.DaemonSet, updates []ImageUpdate) (*UpdateResult, error) {
	result := &UpdateResult{
		Updates:        updates,
		PreviousImages: make(map[string]string),
	}

	// Store previous images
	for _, container := range daemonset.Spec.Template.Spec.Containers {
		result.PreviousImages[container.Name] = container.Image
	}
	for _, container := range daemonset.Spec.Template.Spec.InitContainers {
		result.PreviousImages["init:"+container.Name] = container.Image
	}

	// Create a copy for patching
	patch := daemonset.DeepCopy()

	// Apply updates
	for _, update := range updates {
		if update.IsInitContainer {
			for i := range patch.Spec.Template.Spec.InitContainers {
				if patch.Spec.Template.Spec.InitContainers[i].Name == update.ContainerName {
					patch.Spec.Template.Spec.InitContainers[i].Image = update.NewImage
					break
				}
			}
		} else {
			for i := range patch.Spec.Template.Spec.Containers {
				if patch.Spec.Template.Spec.Containers[i].Name == update.ContainerName {
					patch.Spec.Template.Spec.Containers[i].Image = update.NewImage
					break
				}
			}
		}
	}

	// Update annotations
	if patch.Annotations == nil {
		patch.Annotations = make(map[string]string)
	}
	patch.Annotations[annotations.StatusLastUpdate] = time.Now().UTC().Format(time.RFC3339)
	patch.Annotations[annotations.PreviousImage] = formatPreviousImages(result.PreviousImages)

	count := 0
	if v, ok := daemonset.Annotations[annotations.StatusUpdateCount]; ok {
		fmt.Sscanf(v, "%d", &count)
	}
	patch.Annotations[annotations.StatusUpdateCount] = fmt.Sprintf("%d", count+1)

	delete(patch.Annotations, annotations.ApprovedBy)
	delete(patch.Annotations, annotations.ApprovedImage)
	delete(patch.Annotations, annotations.StatusAvailableUpdate)

	if err := p.client.Patch(ctx, patch, client.MergeFrom(daemonset)); err != nil {
		result.Error = fmt.Errorf("failed to patch daemonset: %w", err)
		return result, result.Error
	}

	result.Success = true
	return result, nil
}

// PatchStatefulSet updates a StatefulSet with new images
func (p *Patcher) PatchStatefulSet(ctx context.Context, statefulset *appsv1.StatefulSet, updates []ImageUpdate) (*UpdateResult, error) {
	result := &UpdateResult{
		Updates:        updates,
		PreviousImages: make(map[string]string),
	}

	// Store previous images
	for _, container := range statefulset.Spec.Template.Spec.Containers {
		result.PreviousImages[container.Name] = container.Image
	}
	for _, container := range statefulset.Spec.Template.Spec.InitContainers {
		result.PreviousImages["init:"+container.Name] = container.Image
	}

	// Create a copy for patching
	patch := statefulset.DeepCopy()

	// Apply updates
	for _, update := range updates {
		if update.IsInitContainer {
			for i := range patch.Spec.Template.Spec.InitContainers {
				if patch.Spec.Template.Spec.InitContainers[i].Name == update.ContainerName {
					patch.Spec.Template.Spec.InitContainers[i].Image = update.NewImage
					break
				}
			}
		} else {
			for i := range patch.Spec.Template.Spec.Containers {
				if patch.Spec.Template.Spec.Containers[i].Name == update.ContainerName {
					patch.Spec.Template.Spec.Containers[i].Image = update.NewImage
					break
				}
			}
		}
	}

	// Update annotations
	if patch.Annotations == nil {
		patch.Annotations = make(map[string]string)
	}
	patch.Annotations[annotations.StatusLastUpdate] = time.Now().UTC().Format(time.RFC3339)
	patch.Annotations[annotations.PreviousImage] = formatPreviousImages(result.PreviousImages)

	count := 0
	if v, ok := statefulset.Annotations[annotations.StatusUpdateCount]; ok {
		fmt.Sscanf(v, "%d", &count)
	}
	patch.Annotations[annotations.StatusUpdateCount] = fmt.Sprintf("%d", count+1)

	delete(patch.Annotations, annotations.ApprovedBy)
	delete(patch.Annotations, annotations.ApprovedImage)
	delete(patch.Annotations, annotations.StatusAvailableUpdate)

	if err := p.client.Patch(ctx, patch, client.MergeFrom(statefulset)); err != nil {
		result.Error = fmt.Errorf("failed to patch statefulset: %w", err)
		return result, result.Error
	}

	result.Success = true
	return result, nil
}

// SetAvailableUpdate sets the available update annotation (for approval workflow)
func (p *Patcher) SetAvailableUpdate(ctx context.Context, obj client.Object, availableImage string) error {
	patch := obj.DeepCopyObject().(client.Object)

	ann := patch.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}
	ann[annotations.StatusAvailableUpdate] = availableImage
	ann[annotations.StatusLastCheck] = time.Now().UTC().Format(time.RFC3339)
	patch.SetAnnotations(ann)

	return p.client.Patch(ctx, patch, client.MergeFrom(obj))
}

// SetError sets an error annotation
func (p *Patcher) SetError(ctx context.Context, obj client.Object, errMsg string) error {
	patch := obj.DeepCopyObject().(client.Object)

	ann := patch.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}
	ann[annotations.StatusError] = errMsg
	ann[annotations.StatusLastCheck] = time.Now().UTC().Format(time.RFC3339)
	patch.SetAnnotations(ann)

	return p.client.Patch(ctx, patch, client.MergeFrom(obj))
}

// ClearError clears the error annotation
func (p *Patcher) ClearError(ctx context.Context, obj client.Object) error {
	ann := obj.GetAnnotations()
	if ann == nil || ann[annotations.StatusError] == "" {
		return nil
	}

	patch := obj.DeepCopyObject().(client.Object)
	patchAnn := patch.GetAnnotations()
	delete(patchAnn, annotations.StatusError)
	patch.SetAnnotations(patchAnn)

	return p.client.Patch(ctx, patch, client.MergeFrom(obj))
}

// ExtractContainers extracts containers from a pod spec
func ExtractContainers(podSpec *corev1.PodSpec) (containers, initContainers []corev1.Container) {
	return podSpec.Containers, podSpec.InitContainers
}

// GetWorkloadPodSpec returns the pod spec from a workload
func GetWorkloadPodSpec(obj metav1.Object) *corev1.PodSpec {
	switch w := obj.(type) {
	case *appsv1.Deployment:
		return &w.Spec.Template.Spec
	case *appsv1.DaemonSet:
		return &w.Spec.Template.Spec
	case *appsv1.StatefulSet:
		return &w.Spec.Template.Spec
	default:
		return nil
	}
}

// ParseImageReference parses an image reference into components
func ParseImageReference(image string) (registry, repository, tag, digest string) {
	// Handle digest
	if idx := strings.Index(image, "@"); idx != -1 {
		digest = image[idx+1:]
		image = image[:idx]
	}

	// Handle tag
	if idx := strings.LastIndex(image, ":"); idx != -1 {
		// Make sure it's a tag, not a port
		afterColon := image[idx+1:]
		if !strings.Contains(afterColon, "/") {
			tag = afterColon
			image = image[:idx]
		}
	}

	// Split registry and repository
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		// No registry, just repository (e.g., "nginx")
		repository = parts[0]
		registry = "docker.io"
	} else if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
		// First part looks like a registry
		registry = parts[0]
		repository = parts[1]
	} else {
		// First part is part of the repository (e.g., "library/nginx")
		registry = "docker.io"
		repository = image
	}

	return
}

// BuildImageReference builds an image reference from components
func BuildImageReference(registry, repository, tag, digest string, strategy string) string {
	var image string

	if registry != "" && registry != "docker.io" {
		image = registry + "/" + repository
	} else {
		image = repository
	}

	switch strategy {
	case "tag":
		if tag != "" {
			image += ":" + tag
		}
	case "digest":
		if digest != "" {
			image += "@" + digest
		}
	case "tag+digest":
		fallthrough
	default:
		if tag != "" {
			image += ":" + tag
		}
		if digest != "" {
			image += "@" + digest
		}
	}

	return image
}

// formatPreviousImages formats previous images for annotation storage
func formatPreviousImages(images map[string]string) string {
	var parts []string
	for name, image := range images {
		parts = append(parts, name+"="+image)
	}
	return strings.Join(parts, ",")
}

// ParsePreviousImages parses the previous images annotation
func ParsePreviousImages(value string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(value, ",") {
		if idx := strings.Index(part, "="); idx != -1 {
			result[part[:idx]] = part[idx+1:]
		}
	}
	return result
}
