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

package ecr

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"

	"github.com/bcfmtolgahan/lookout-operator/pkg/registry"
)

// ECRRegistry implements the Registry interface for AWS ECR
type ECRRegistry struct {
	name      string
	region    string
	accountID string
	client    *ecr.Client
	endpoint  string
}

// NewECRRegistry creates a new ECR registry client
func NewECRRegistry(ctx context.Context, cfg *registry.Config) (*ECRRegistry, error) {
	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, registry.NewAuthError("failed to load AWS config", err)
	}

	client := ecr.NewFromConfig(awsCfg)

	// Get account ID if not provided
	accountID := cfg.Project
	if accountID == "" {
		// Try to get account ID from authorization token
		authOutput, err := client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
		if err != nil {
			return nil, registry.NewAuthError("failed to get ECR authorization token", err)
		}
		if len(authOutput.AuthorizationData) > 0 && authOutput.AuthorizationData[0].ProxyEndpoint != nil {
			// Parse account ID from proxy endpoint
			// Format: https://<account-id>.dkr.ecr.<region>.amazonaws.com
			endpoint := *authOutput.AuthorizationData[0].ProxyEndpoint
			// Extract account ID (simplified, in production use proper parsing)
			accountID = extractAccountID(endpoint)
		}
	}

	endpoint := fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", accountID, cfg.Region)

	return &ECRRegistry{
		name:      cfg.Name,
		region:    cfg.Region,
		accountID: accountID,
		client:    client,
		endpoint:  endpoint,
	}, nil
}

// Type returns the registry type
func (r *ECRRegistry) Type() string {
	return "ecr"
}

// Name returns the registry name
func (r *ECRRegistry) Name() string {
	return r.name
}

// Endpoint returns the registry endpoint
func (r *ECRRegistry) Endpoint() string {
	return r.endpoint
}

// ListTags returns all tags for a repository
func (r *ECRRegistry) ListTags(ctx context.Context, repository string, opts registry.ListOptions) ([]registry.Image, error) {
	var images []registry.Image
	var nextToken *string

	for {
		input := &ecr.DescribeImagesInput{
			RepositoryName: aws.String(repository),
			MaxResults:     aws.Int32(100),
			NextToken:      nextToken,
		}

		output, err := r.client.DescribeImages(ctx, input)
		if err != nil {
			return nil, r.handleError("list tags", err)
		}

		for _, img := range output.ImageDetails {
			// Skip untagged images
			if len(img.ImageTags) == 0 {
				continue
			}

			for _, tag := range img.ImageTags {
				pushedAt := time.Time{}
				if img.ImagePushedAt != nil {
					pushedAt = *img.ImagePushedAt
				}

				var size int64
				if img.ImageSizeInBytes != nil {
					size = *img.ImageSizeInBytes
				}

				digest := ""
				if img.ImageDigest != nil {
					digest = *img.ImageDigest
				}

				fullRef := fmt.Sprintf("%s/%s:%s", r.endpoint, repository, tag)
				if digest != "" {
					fullRef = fmt.Sprintf("%s/%s:%s@%s", r.endpoint, repository, tag, digest)
				}

				images = append(images, registry.Image{
					Repository:    repository,
					Tag:           tag,
					Digest:        digest,
					PushedAt:      pushedAt,
					Size:          size,
					FullReference: fullRef,
				})
			}
		}

		nextToken = output.NextToken
		if nextToken == nil {
			break
		}

		// Check if we've reached the limit
		if opts.Limit > 0 && len(images) >= opts.Limit {
			break
		}
	}

	// Sort images
	sortImages(images, opts.SortBy, opts.SortOrder)

	// Apply limit
	if opts.Limit > 0 && len(images) > opts.Limit {
		images = images[:opts.Limit]
	}

	return images, nil
}

// GetDigest returns the digest for a specific tag
func (r *ECRRegistry) GetDigest(ctx context.Context, repository, tag string) (string, error) {
	input := &ecr.DescribeImagesInput{
		RepositoryName: aws.String(repository),
		ImageIds: []types.ImageIdentifier{
			{
				ImageTag: aws.String(tag),
			},
		},
	}

	output, err := r.client.DescribeImages(ctx, input)
	if err != nil {
		return "", r.handleError("get digest", err)
	}

	if len(output.ImageDetails) == 0 {
		return "", registry.NewNotFoundError(
			fmt.Sprintf("image not found: %s:%s", repository, tag),
			nil,
		)
	}

	if output.ImageDetails[0].ImageDigest != nil {
		return *output.ImageDetails[0].ImageDigest, nil
	}

	return "", registry.NewNotFoundError("digest not available", nil)
}

// ImageExists checks if an image exists
func (r *ECRRegistry) ImageExists(ctx context.Context, repository, tag string) (bool, error) {
	_, err := r.GetDigest(ctx, repository, tag)
	if err != nil {
		if regErr, ok := err.(*registry.RegistryError); ok {
			if regErr.Code == registry.ErrCodeNotFound {
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}

// CheckHealth verifies connectivity and authentication
func (r *ECRRegistry) CheckHealth(ctx context.Context) error {
	_, err := r.client.GetAuthorizationToken(ctx, &ecr.GetAuthorizationTokenInput{})
	if err != nil {
		return registry.NewAuthError("ECR health check failed", err)
	}
	return nil
}

// Close cleans up resources
func (r *ECRRegistry) Close() error {
	// AWS SDK v2 doesn't require explicit cleanup
	return nil
}

// handleError converts AWS errors to registry errors
func (r *ECRRegistry) handleError(operation string, err error) error {
	if err == nil {
		return nil
	}

	// Check for specific error types
	// In production, check for specific AWS error codes
	errMsg := err.Error()

	// Rate limit check
	if contains(errMsg, "ThrottlingException", "RequestLimitExceeded") {
		return registry.NewRateLimitError(
			fmt.Sprintf("ECR %s rate limited", operation),
			err,
		)
	}

	// Auth check
	if contains(errMsg, "AccessDenied", "UnauthorizedAccess", "ExpiredToken") {
		return registry.NewAuthError(
			fmt.Sprintf("ECR %s unauthorized", operation),
			err,
		)
	}

	// Not found check
	if contains(errMsg, "RepositoryNotFoundException", "ImageNotFoundException") {
		return registry.NewNotFoundError(
			fmt.Sprintf("ECR %s not found", operation),
			err,
		)
	}

	// Network/timeout
	if contains(errMsg, "timeout", "connection refused", "no such host") {
		return registry.NewNetworkError(
			fmt.Sprintf("ECR %s network error", operation),
			err,
		)
	}

	// Generic error
	return &registry.RegistryError{
		Code:       registry.ErrCodeUnknown,
		Message:    fmt.Sprintf("ECR %s failed", operation),
		Retryable:  false,
		Underlying: err,
	}
}

// sortImages sorts images based on options
func sortImages(images []registry.Image, sortBy, sortOrder string) {
	sort.Slice(images, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "tagName":
			less = images[i].Tag < images[j].Tag
		case "pushedAt":
			fallthrough
		default:
			less = images[i].PushedAt.Before(images[j].PushedAt)
		}

		if sortOrder == "desc" {
			return !less
		}
		return less
	})
}

// extractAccountID extracts account ID from ECR endpoint
func extractAccountID(endpoint string) string {
	// Format: https://<account-id>.dkr.ecr.<region>.amazonaws.com
	// Simple extraction - in production use proper URL parsing
	if len(endpoint) > 8 {
		endpoint = endpoint[8:] // Remove https://
		for i, c := range endpoint {
			if c == '.' {
				return endpoint[:i]
			}
		}
	}
	return ""
}

// contains checks if any of the substrings are in the string
func contains(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(substr) > 0 {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

// Creator creates an ECR registry from config
func Creator(ctx context.Context, cfg *registry.Config) (registry.Registry, error) {
	return NewECRRegistry(ctx, cfg)
}
