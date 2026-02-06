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

package registry

import (
	"context"
	"time"
)

// Image represents a container image with its metadata
type Image struct {
	// Repository is the repository name (without registry prefix)
	Repository string `json:"repository"`

	// Tag is the image tag
	Tag string `json:"tag"`

	// Digest is the image digest (sha256:...)
	Digest string `json:"digest"`

	// PushedAt is when the image was pushed to the registry
	PushedAt time.Time `json:"pushedAt"`

	// Size is the image size in bytes
	Size int64 `json:"size,omitempty"`

	// FullReference is the complete image reference
	// Format: registry/repository:tag@digest
	FullReference string `json:"fullReference"`
}

// ListOptions defines options for listing tags
type ListOptions struct {
	// Limit is the maximum number of tags to return
	Limit int

	// SortBy defines how to sort the results
	// Values: "pushedAt", "tagName"
	SortBy string

	// SortOrder defines the sort direction
	// Values: "asc", "desc"
	SortOrder string
}

// DefaultListOptions returns default list options
func DefaultListOptions() ListOptions {
	return ListOptions{
		Limit:     100,
		SortBy:    "pushedAt",
		SortOrder: "desc",
	}
}

// Registry is the interface that all registry implementations must satisfy
type Registry interface {
	// Type returns the registry type identifier (ecr, harbor, dockerhub, etc.)
	Type() string

	// Name returns the registry name (from ImageRegistry CRD)
	Name() string

	// Endpoint returns the registry endpoint URL
	Endpoint() string

	// ListTags returns all tags for a repository
	ListTags(ctx context.Context, repository string, opts ListOptions) ([]Image, error)

	// GetDigest returns the digest for a specific tag
	GetDigest(ctx context.Context, repository, tag string) (string, error)

	// ImageExists checks if an image exists in the registry
	ImageExists(ctx context.Context, repository, tag string) (bool, error)

	// CheckHealth verifies connectivity and authentication to the registry
	CheckHealth(ctx context.Context) error

	// Close cleans up any resources (connections, tokens, etc.)
	Close() error
}

// Credentials holds authentication credentials for a registry
type Credentials struct {
	// Username for basic auth
	Username string

	// Password or token for basic auth
	Password string

	// Token for bearer token auth
	Token string

	// ServiceAccountJSON for GCR
	ServiceAccountJSON []byte

	// UseWorkloadIdentity indicates to use cloud workload identity
	UseWorkloadIdentity bool
}

// Config holds configuration for creating a registry client
type Config struct {
	// Name is the registry name (from CRD)
	Name string

	// Type is the registry type
	Type string

	// Endpoint is the registry endpoint URL
	Endpoint string

	// Region is the cloud region (for ECR, GCR, ACR)
	Region string

	// Project is the project/account ID (for GCR, ACR)
	Project string

	// Credentials holds auth credentials
	Credentials *Credentials

	// InsecureSkipVerify skips TLS verification
	InsecureSkipVerify bool

	// CABundle is custom CA certificate
	CABundle []byte
}

// RegistryError represents a registry-specific error
type RegistryError struct {
	// Code is the error code
	Code string

	// Message is the error message
	Message string

	// Retryable indicates if the error is retryable
	Retryable bool

	// Underlying is the underlying error
	Underlying error
}

func (e *RegistryError) Error() string {
	if e.Underlying != nil {
		return e.Message + ": " + e.Underlying.Error()
	}
	return e.Message
}

func (e *RegistryError) Unwrap() error {
	return e.Underlying
}

// Error codes
const (
	ErrCodeAuth         = "AUTH_ERROR"
	ErrCodeNotFound     = "NOT_FOUND"
	ErrCodeRateLimit    = "RATE_LIMIT"
	ErrCodeNetwork      = "NETWORK_ERROR"
	ErrCodeTimeout      = "TIMEOUT"
	ErrCodeInvalidImage = "INVALID_IMAGE"
	ErrCodeUnknown      = "UNKNOWN"
)

// NewAuthError creates an authentication error
func NewAuthError(msg string, err error) *RegistryError {
	return &RegistryError{
		Code:       ErrCodeAuth,
		Message:    msg,
		Retryable:  false,
		Underlying: err,
	}
}

// NewNotFoundError creates a not found error
func NewNotFoundError(msg string, err error) *RegistryError {
	return &RegistryError{
		Code:       ErrCodeNotFound,
		Message:    msg,
		Retryable:  false,
		Underlying: err,
	}
}

// NewRateLimitError creates a rate limit error
func NewRateLimitError(msg string, err error) *RegistryError {
	return &RegistryError{
		Code:       ErrCodeRateLimit,
		Message:    msg,
		Retryable:  true,
		Underlying: err,
	}
}

// NewNetworkError creates a network error
func NewNetworkError(msg string, err error) *RegistryError {
	return &RegistryError{
		Code:       ErrCodeNetwork,
		Message:    msg,
		Retryable:  true,
		Underlying: err,
	}
}

// NewTimeoutError creates a timeout error
func NewTimeoutError(msg string, err error) *RegistryError {
	return &RegistryError{
		Code:       ErrCodeTimeout,
		Message:    msg,
		Retryable:  true,
		Underlying: err,
	}
}
