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

package cache

import (
	"sync"
	"time"

	"github.com/bcfmtolgahan/lookout-operator/pkg/registry"
)

// RepositoryCache holds cached images for a repository
type RepositoryCache struct {
	// Images is the list of cached images
	Images []registry.Image

	// LastScanAt is when the cache was last updated
	LastScanAt time.Time

	// ScanError is the last scan error (if any)
	ScanError error

	// TTL is the cache time-to-live
	TTL time.Duration
}

// IsExpired checks if the cache has expired
func (c *RepositoryCache) IsExpired() bool {
	if c.TTL == 0 {
		return false
	}
	return time.Since(c.LastScanAt) > c.TTL
}

// ImageCache is a thread-safe cache for container images
type ImageCache struct {
	mu    sync.RWMutex
	cache map[string]map[string]*RepositoryCache // registry -> repository -> cache
}

// NewImageCache creates a new image cache
func NewImageCache() *ImageCache {
	return &ImageCache{
		cache: make(map[string]map[string]*RepositoryCache),
	}
}

// Get retrieves cached images for a repository
func (c *ImageCache) Get(registryName, repository string) (*RepositoryCache, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if repos, ok := c.cache[registryName]; ok {
		if cache, ok := repos[repository]; ok {
			return cache, true
		}
	}

	return nil, false
}

// Set stores images in the cache
func (c *ImageCache) Set(registryName, repository string, images []registry.Image, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.cache[registryName]; !ok {
		c.cache[registryName] = make(map[string]*RepositoryCache)
	}

	c.cache[registryName][repository] = &RepositoryCache{
		Images:     images,
		LastScanAt: time.Now(),
		ScanError:  nil,
		TTL:        ttl,
	}
}

// SetError stores a scan error in the cache
func (c *ImageCache) SetError(registryName, repository string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.cache[registryName]; !ok {
		c.cache[registryName] = make(map[string]*RepositoryCache)
	}

	// Keep existing images if available
	existing := c.cache[registryName][repository]
	var images []registry.Image
	if existing != nil {
		images = existing.Images
	}

	c.cache[registryName][repository] = &RepositoryCache{
		Images:     images,
		LastScanAt: time.Now(),
		ScanError:  err,
	}
}

// Delete removes a repository from the cache
func (c *ImageCache) Delete(registryName, repository string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if repos, ok := c.cache[registryName]; ok {
		delete(repos, repository)
	}
}

// DeleteRegistry removes all repositories for a registry
func (c *ImageCache) DeleteRegistry(registryName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, registryName)
}

// GetRegistryStats returns stats for a registry
func (c *ImageCache) GetRegistryStats(registryName string) (repoCount, tagCount int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if repos, ok := c.cache[registryName]; ok {
		repoCount = len(repos)
		for _, cache := range repos {
			tagCount += len(cache.Images)
		}
	}

	return
}

// ListRepositories returns all repositories for a registry
func (c *ImageCache) ListRepositories(registryName string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var repos []string
	if repoCache, ok := c.cache[registryName]; ok {
		for repo := range repoCache {
			repos = append(repos, repo)
		}
	}

	return repos
}

// Clear clears the entire cache
func (c *ImageCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]map[string]*RepositoryCache)
}

// FindImage finds a specific image by tag or digest
func (c *ImageCache) FindImage(registryName, repository, tagOrDigest string) (*registry.Image, bool) {
	cache, ok := c.Get(registryName, repository)
	if !ok {
		return nil, false
	}

	for _, img := range cache.Images {
		if img.Tag == tagOrDigest || img.Digest == tagOrDigest {
			return &img, true
		}
	}

	return nil, false
}

// GetLatestImage returns the latest image based on push time
func (c *ImageCache) GetLatestImage(registryName, repository string) (*registry.Image, bool) {
	cache, ok := c.Get(registryName, repository)
	if !ok || len(cache.Images) == 0 {
		return nil, false
	}

	// Find latest by push time
	var latest *registry.Image
	for i := range cache.Images {
		if latest == nil || cache.Images[i].PushedAt.After(latest.PushedAt) {
			latest = &cache.Images[i]
		}
	}

	return latest, latest != nil
}
