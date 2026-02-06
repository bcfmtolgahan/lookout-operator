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

package policy

import (
	"sort"

	"github.com/bcfmtolgahan/lookout-operator/pkg/registry"
)

// DigestPolicy selects images based on digest changes
// This is useful when you want to track mutable tags like "latest"
type DigestPolicy struct {
	cfg *Config
}

// NewDigestPolicy creates a new digest policy
func NewDigestPolicy(cfg *Config) *DigestPolicy {
	return &DigestPolicy{cfg: cfg}
}

// Name returns the policy name
func (p *DigestPolicy) Name() string {
	return "digest"
}

// Select chooses the most recent image by push time
// The digest policy focuses on digest comparison during update checks
func (p *DigestPolicy) Select(candidates []registry.Image) *registry.Image {
	filtered := p.Filter(candidates)
	if len(filtered) == 0 {
		return nil
	}

	// Sort by push time descending
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].PushedAt.After(filtered[j].PushedAt)
	})

	return &filtered[0]
}

// Filter filters images - for digest policy, we include all images with digests
func (p *DigestPolicy) Filter(images []registry.Image) []registry.Image {
	var filtered []registry.Image

	for _, img := range images {
		// Only include images with digests
		if img.Digest == "" {
			continue
		}

		// Check exclude tags
		excluded := false
		for _, pattern := range p.cfg.ExcludeTags {
			if matchGlob(pattern, img.Tag) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		// Check include tags (if specified)
		if len(p.cfg.IncludeTags) > 0 {
			included := false
			for _, pattern := range p.cfg.IncludeTags {
				if matchGlob(pattern, img.Tag) {
					included = true
					break
				}
			}
			if !included {
				continue
			}
		}

		filtered = append(filtered, img)
	}

	return filtered
}

// DigestChanged checks if the digest has changed for the same tag
func DigestChanged(oldDigest, newDigest string) bool {
	if oldDigest == "" || newDigest == "" {
		return false
	}
	return oldDigest != newDigest
}
