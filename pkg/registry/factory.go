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
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	registryv1alpha1 "github.com/bcfmtolgahan/lookout-operator/api/v1alpha1"
)

// Factory creates and manages registry clients
type Factory struct {
	client    client.Client
	mu        sync.RWMutex
	clients   map[string]Registry
	creators  map[registryv1alpha1.RegistryType]CreatorFunc
}

// CreatorFunc is a function that creates a Registry
type CreatorFunc func(ctx context.Context, cfg *Config) (Registry, error)

// NewFactory creates a new registry factory
func NewFactory(k8sClient client.Client) *Factory {
	f := &Factory{
		client:   k8sClient,
		clients:  make(map[string]Registry),
		creators: make(map[registryv1alpha1.RegistryType]CreatorFunc),
	}

	// Register built-in creators
	// ECR creator will be registered in the ecr package init

	return f
}

// RegisterCreator registers a creator function for a registry type
func (f *Factory) RegisterCreator(regType registryv1alpha1.RegistryType, creator CreatorFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creators[regType] = creator
}

// GetOrCreate gets an existing client or creates a new one
func (f *Factory) GetOrCreate(ctx context.Context, reg *registryv1alpha1.ImageRegistry) (Registry, error) {
	key := fmt.Sprintf("%s/%s", reg.Namespace, reg.Name)

	// Try to get existing client
	f.mu.RLock()
	if client, ok := f.clients[key]; ok {
		f.mu.RUnlock()
		return client, nil
	}
	f.mu.RUnlock()

	// Create new client
	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check after acquiring write lock
	if client, ok := f.clients[key]; ok {
		return client, nil
	}

	// Build config
	cfg, err := f.buildConfig(ctx, reg)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	// Get creator for this type
	creator, ok := f.creators[reg.Spec.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported registry type: %s", reg.Spec.Type)
	}

	// Create client
	client, err := creator(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create registry client: %w", err)
	}

	f.clients[key] = client
	return client, nil
}

// Remove removes a client from the cache
func (f *Factory) Remove(namespace, name string) error {
	key := fmt.Sprintf("%s/%s", namespace, name)

	f.mu.Lock()
	defer f.mu.Unlock()

	if client, ok := f.clients[key]; ok {
		if err := client.Close(); err != nil {
			return err
		}
		delete(f.clients, key)
	}

	return nil
}

// Close closes all registry clients
func (f *Factory) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var lastErr error
	for key, client := range f.clients {
		if err := client.Close(); err != nil {
			lastErr = err
		}
		delete(f.clients, key)
	}

	return lastErr
}

// buildConfig builds a registry config from an ImageRegistry CRD
func (f *Factory) buildConfig(ctx context.Context, reg *registryv1alpha1.ImageRegistry) (*Config, error) {
	cfg := &Config{
		Name: reg.Name,
		Type: string(reg.Spec.Type),
	}

	// Set type-specific config
	switch reg.Spec.Type {
	case registryv1alpha1.RegistryTypeECR:
		if reg.Spec.ECR != nil {
			cfg.Region = reg.Spec.ECR.Region
			cfg.Project = reg.Spec.ECR.AccountID
			cfg.Endpoint = fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com",
				reg.Spec.ECR.AccountID, reg.Spec.ECR.Region)
		}

	case registryv1alpha1.RegistryTypeHarbor:
		if reg.Spec.Harbor != nil {
			cfg.Endpoint = reg.Spec.Harbor.URL
			cfg.InsecureSkipVerify = reg.Spec.Harbor.InsecureSkipVerify
		}

	case registryv1alpha1.RegistryTypeDockerHub:
		cfg.Endpoint = "https://registry-1.docker.io"
		if reg.Spec.DockerHub != nil && reg.Spec.DockerHub.Organization != "" {
			cfg.Project = reg.Spec.DockerHub.Organization
		}

	case registryv1alpha1.RegistryTypeGCR:
		if reg.Spec.GCR != nil {
			location := "gcr.io"
			if reg.Spec.GCR.Location != "" && reg.Spec.GCR.Location != "us" {
				location = fmt.Sprintf("%s.gcr.io", reg.Spec.GCR.Location)
			}
			cfg.Endpoint = fmt.Sprintf("https://%s", location)
			cfg.Project = reg.Spec.GCR.Project
		}

	case registryv1alpha1.RegistryTypeACR:
		if reg.Spec.ACR != nil {
			cfg.Endpoint = fmt.Sprintf("https://%s.azurecr.io", reg.Spec.ACR.RegistryName)
			cfg.Project = reg.Spec.ACR.SubscriptionID
		}

	case registryv1alpha1.RegistryTypeGeneric:
		if reg.Spec.Generic != nil {
			cfg.Endpoint = reg.Spec.Generic.URL
		}
	}

	// Get credentials
	creds, err := f.getCredentials(ctx, reg)
	if err != nil {
		return nil, err
	}
	cfg.Credentials = creds

	// TLS config
	if reg.Spec.TLS != nil {
		cfg.InsecureSkipVerify = reg.Spec.TLS.InsecureSkipVerify
		if reg.Spec.TLS.CASecretRef != nil {
			caBundle, err := f.getCABundle(ctx, reg.Namespace, reg.Spec.TLS.CASecretRef)
			if err != nil {
				return nil, err
			}
			cfg.CABundle = caBundle
		}
	}

	return cfg, nil
}

// getCredentials retrieves credentials for a registry
func (f *Factory) getCredentials(ctx context.Context, reg *registryv1alpha1.ImageRegistry) (*Credentials, error) {
	if reg.Spec.Auth == nil {
		return &Credentials{UseWorkloadIdentity: true}, nil
	}

	creds := &Credentials{}

	// Workload Identity
	if reg.Spec.Auth.WorkloadIdentity != nil && reg.Spec.Auth.WorkloadIdentity.Enabled {
		creds.UseWorkloadIdentity = true
		return creds, nil
	}

	// Secret reference
	if reg.Spec.Auth.SecretRef != nil {
		namespace := reg.Spec.Auth.SecretRef.Namespace
		if namespace == "" {
			namespace = reg.Namespace
		}

		secret := &corev1.Secret{}
		if err := f.client.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      reg.Spec.Auth.SecretRef.Name,
		}, secret); err != nil {
			return nil, fmt.Errorf("failed to get auth secret: %w", err)
		}

		// Parse secret based on type
		switch secret.Type {
		case corev1.SecretTypeDockerConfigJson:
			// Docker config JSON format
			if data, ok := secret.Data[".dockerconfigjson"]; ok {
				creds.Token = string(data)
			}

		case corev1.SecretTypeBasicAuth:
			creds.Username = string(secret.Data["username"])
			creds.Password = string(secret.Data["password"])

		default:
			// Try basic auth format
			if username, ok := secret.Data["username"]; ok {
				creds.Username = string(username)
			}
			if password, ok := secret.Data["password"]; ok {
				creds.Password = string(password)
			}
			if token, ok := secret.Data["token"]; ok {
				creds.Token = string(token)
			}
			// For GCR service account
			if key := reg.Spec.Auth.SecretRef.Key; key != "" {
				if data, ok := secret.Data[key]; ok {
					creds.ServiceAccountJSON = data
				}
			}
		}
	}

	return creds, nil
}

// getCABundle retrieves CA bundle from a secret
func (f *Factory) getCABundle(ctx context.Context, defaultNamespace string, ref *registryv1alpha1.SecretReference) ([]byte, error) {
	namespace := ref.Namespace
	if namespace == "" {
		namespace = defaultNamespace
	}

	secret := &corev1.Secret{}
	if err := f.client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      ref.Name,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get CA secret: %w", err)
	}

	key := ref.Key
	if key == "" {
		key = "ca.crt"
	}

	if data, ok := secret.Data[key]; ok {
		return data, nil
	}

	return nil, fmt.Errorf("CA certificate not found in secret at key %s", key)
}
