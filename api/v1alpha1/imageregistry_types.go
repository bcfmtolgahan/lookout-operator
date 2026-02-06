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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegistryType defines the type of container registry
// +kubebuilder:validation:Enum=ecr;harbor;dockerhub;gcr;acr;quay;generic
type RegistryType string

const (
	RegistryTypeECR       RegistryType = "ecr"
	RegistryTypeHarbor    RegistryType = "harbor"
	RegistryTypeDockerHub RegistryType = "dockerhub"
	RegistryTypeGCR       RegistryType = "gcr"
	RegistryTypeACR       RegistryType = "acr"
	RegistryTypeQuay      RegistryType = "quay"
	RegistryTypeGeneric   RegistryType = "generic"
)

// CircuitBreakerState represents the state of circuit breaker
type CircuitBreakerState string

const (
	CircuitBreakerStateClosed   CircuitBreakerState = "Closed"
	CircuitBreakerStateOpen     CircuitBreakerState = "Open"
	CircuitBreakerStateHalfOpen CircuitBreakerState = "HalfOpen"
)

// ECRConfig defines ECR-specific configuration
type ECRConfig struct {
	// Region is the AWS region where ECR is located
	// +kubebuilder:validation:Required
	Region string `json:"region"`

	// AccountID is the AWS account ID (optional, auto-detected if not specified)
	// +optional
	AccountID string `json:"accountId,omitempty"`
}

// HarborConfig defines Harbor-specific configuration
type HarborConfig struct {
	// URL is the Harbor server URL
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	URL string `json:"url"`

	// Project is the default Harbor project
	// +optional
	Project string `json:"project,omitempty"`

	// InsecureSkipVerify skips TLS verification (not recommended for production)
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// DockerHubConfig defines DockerHub-specific configuration
type DockerHubConfig struct {
	// Organization is the DockerHub organization (optional)
	// +optional
	Organization string `json:"organization,omitempty"`
}

// GCRConfig defines Google Container Registry configuration
type GCRConfig struct {
	// Project is the GCP project ID
	// +kubebuilder:validation:Required
	Project string `json:"project"`

	// Location is the GCR location (eu, us, asia, or specific region)
	// +optional
	// +kubebuilder:default="us"
	Location string `json:"location,omitempty"`
}

// ACRConfig defines Azure Container Registry configuration
type ACRConfig struct {
	// SubscriptionID is the Azure subscription ID
	// +kubebuilder:validation:Required
	SubscriptionID string `json:"subscriptionId"`

	// ResourceGroup is the Azure resource group
	// +kubebuilder:validation:Required
	ResourceGroup string `json:"resourceGroup"`

	// RegistryName is the ACR registry name
	// +kubebuilder:validation:Required
	RegistryName string `json:"registryName"`
}

// GenericConfig defines configuration for generic OCI registries
type GenericConfig struct {
	// URL is the registry URL
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https?://`
	URL string `json:"url"`

	// APIVersion is the Docker Registry API version
	// +optional
	// +kubebuilder:default="v2"
	APIVersion string `json:"apiVersion,omitempty"`
}

// SecretReference references a Kubernetes secret
type SecretReference struct {
	// Name is the name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the secret (defaults to ImageRegistry namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Key is the specific key in the secret (for service account JSON, etc.)
	// +optional
	Key string `json:"key,omitempty"`
}

// WorkloadIdentityConfig defines cloud-native workload identity settings
type WorkloadIdentityConfig struct {
	// Enabled enables workload identity (IRSA for AWS, Workload Identity for GCP, etc.)
	// +optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
}

// AuthConfig defines registry authentication configuration
type AuthConfig struct {
	// WorkloadIdentity uses cloud-native workload identity (IRSA, GCP Workload Identity, etc.)
	// +optional
	WorkloadIdentity *WorkloadIdentityConfig `json:"workloadIdentity,omitempty"`

	// SecretRef references a Kubernetes secret containing credentials
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// ServiceAccountRef references a service account for token-based auth
	// +optional
	ServiceAccountRef *SecretReference `json:"serviceAccountRef,omitempty"`
}

// RateLimitingConfig defines rate limiting settings
type RateLimitingConfig struct {
	// RequestsPerSecond is the maximum requests per second
	// +optional
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	RequestsPerSecond int `json:"requestsPerSecond,omitempty"`

	// Burst is the maximum burst size
	// +optional
	// +kubebuilder:default=20
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=200
	Burst int `json:"burst,omitempty"`
}

// CircuitBreakerConfig defines circuit breaker settings
type CircuitBreakerConfig struct {
	// Enabled enables the circuit breaker
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// FailureThreshold is the number of failures before opening the circuit
	// +optional
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	FailureThreshold int `json:"failureThreshold,omitempty"`

	// SuccessThreshold is the number of successes before closing the circuit
	// +optional
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	SuccessThreshold int `json:"successThreshold,omitempty"`

	// Timeout is the time to wait before transitioning from open to half-open
	// +optional
	// +kubebuilder:default="5m"
	Timeout string `json:"timeout,omitempty"`
}

// ScanConfig defines image scanning configuration
type ScanConfig struct {
	// MinInterval is the minimum polling interval (workloads can't go below this)
	// +optional
	// +kubebuilder:default="1m"
	MinInterval string `json:"minInterval,omitempty"`

	// DefaultInterval is the default polling interval
	// +optional
	// +kubebuilder:default="5m"
	DefaultInterval string `json:"defaultInterval,omitempty"`

	// TagLimit is the maximum number of tags to fetch per repository
	// +optional
	// +kubebuilder:default=100
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=1000
	TagLimit int `json:"tagLimit,omitempty"`

	// SortBy defines how to sort tags when fetching
	// +optional
	// +kubebuilder:default="pushedAt"
	// +kubebuilder:validation:Enum=pushedAt;tagName
	SortBy string `json:"sortBy,omitempty"`
}

// TLSConfig defines TLS configuration
type TLSConfig struct {
	// InsecureSkipVerify skips TLS verification (not recommended for production)
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`

	// CASecretRef references a secret containing CA certificate
	// +optional
	CASecretRef *SecretReference `json:"caSecretRef,omitempty"`
}

// ImageRegistrySpec defines the desired state of ImageRegistry
type ImageRegistrySpec struct {
	// Type is the registry type (ecr, harbor, dockerhub, gcr, acr, quay, generic)
	// +kubebuilder:validation:Required
	Type RegistryType `json:"type"`

	// ECR contains ECR-specific configuration
	// +optional
	ECR *ECRConfig `json:"ecr,omitempty"`

	// Harbor contains Harbor-specific configuration
	// +optional
	Harbor *HarborConfig `json:"harbor,omitempty"`

	// DockerHub contains DockerHub-specific configuration
	// +optional
	DockerHub *DockerHubConfig `json:"dockerHub,omitempty"`

	// GCR contains Google Container Registry configuration
	// +optional
	GCR *GCRConfig `json:"gcr,omitempty"`

	// ACR contains Azure Container Registry configuration
	// +optional
	ACR *ACRConfig `json:"acr,omitempty"`

	// Generic contains generic OCI registry configuration
	// +optional
	Generic *GenericConfig `json:"generic,omitempty"`

	// Auth defines authentication configuration
	// +optional
	Auth *AuthConfig `json:"auth,omitempty"`

	// RateLimiting defines rate limiting settings
	// +optional
	RateLimiting *RateLimitingConfig `json:"rateLimiting,omitempty"`

	// CircuitBreaker defines circuit breaker settings
	// +optional
	CircuitBreaker *CircuitBreakerConfig `json:"circuitBreaker,omitempty"`

	// Scan defines image scanning configuration
	// +optional
	Scan *ScanConfig `json:"scan,omitempty"`

	// TLS defines TLS configuration
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`
}

// TrackedRepository represents a repository being tracked
type TrackedRepository struct {
	// Name is the repository name
	Name string `json:"name"`

	// FullPath is the full repository path including registry
	FullPath string `json:"fullPath"`

	// LastScanAt is the last successful scan time
	// +optional
	LastScanAt *metav1.Time `json:"lastScanAt,omitempty"`

	// TagCount is the number of tags found
	// +optional
	TagCount int `json:"tagCount,omitempty"`

	// LatestTag is the latest tag found (according to policy)
	// +optional
	LatestTag string `json:"latestTag,omitempty"`

	// WatcherCount is the number of workloads watching this repository
	// +optional
	WatcherCount int `json:"watcherCount,omitempty"`
}

// CircuitBreakerStatus represents circuit breaker state
type CircuitBreakerStatus struct {
	// State is the current circuit breaker state
	// +optional
	State CircuitBreakerState `json:"state,omitempty"`

	// Failures is the current failure count
	// +optional
	Failures int `json:"failures,omitempty"`

	// LastFailure is the time of the last failure
	// +optional
	LastFailure *metav1.Time `json:"lastFailure,omitempty"`
}

// AuthStatus represents authentication status
type AuthStatus struct {
	// Valid indicates if authentication is valid
	// +optional
	Valid bool `json:"valid,omitempty"`

	// ExpiresAt is when the credentials expire (if known)
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// LastValidated is the last time authentication was validated
	// +optional
	LastValidated *metav1.Time `json:"lastValidated,omitempty"`
}

// ImageRegistryStatus defines the observed state of ImageRegistry
type ImageRegistryStatus struct {
	// Conditions represent the current state of the ImageRegistry
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastConnectedAt is the last successful connection time
	// +optional
	LastConnectedAt *metav1.Time `json:"lastConnectedAt,omitempty"`

	// TrackedRepositories are the repositories being tracked
	// +optional
	TrackedRepositories []TrackedRepository `json:"trackedRepositories,omitempty"`

	// CircuitBreaker is the current circuit breaker status
	// +optional
	CircuitBreaker *CircuitBreakerStatus `json:"circuitBreaker,omitempty"`

	// Auth is the current authentication status
	// +optional
	Auth *AuthStatus `json:"auth,omitempty"`

	// ObservedGeneration is the last observed generation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ir;imgreg
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ImageRegistry is the Schema for the imageregistries API
type ImageRegistry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of ImageRegistry
	// +kubebuilder:validation:Required
	Spec ImageRegistrySpec `json:"spec"`

	// Status defines the observed state of ImageRegistry
	// +optional
	Status ImageRegistryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImageRegistryList contains a list of ImageRegistry
type ImageRegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageRegistry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageRegistry{}, &ImageRegistryList{})
}

// Condition types for ImageRegistry
const (
	// ConditionTypeReady indicates the registry is ready and accessible
	ConditionTypeReady = "Ready"

	// ConditionTypeAuthValid indicates authentication is valid
	ConditionTypeAuthValid = "AuthValid"

	// ConditionTypeAuthExpiringSoon indicates credentials are expiring soon
	ConditionTypeAuthExpiringSoon = "AuthExpiringSoon"

	// ConditionTypeRateLimited indicates the registry is being rate limited
	ConditionTypeRateLimited = "RateLimited"

	// ConditionTypeCircuitOpen indicates the circuit breaker is open
	ConditionTypeCircuitOpen = "CircuitOpen"
)

// Condition reasons
const (
	ReasonConnectionSuccessful = "ConnectionSuccessful"
	ReasonConnectionFailed     = "ConnectionFailed"
	ReasonAuthValid            = "AuthenticationValid"
	ReasonAuthInvalid          = "AuthenticationInvalid"
	ReasonAuthExpiringSoon     = "CredentialsExpiringSoon"
	ReasonRateLimited          = "RateLimitExceeded"
	ReasonCircuitOpen          = "CircuitBreakerOpen"
	ReasonCircuitClosed        = "CircuitBreakerClosed"
)
