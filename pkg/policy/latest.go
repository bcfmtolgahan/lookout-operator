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
	"regexp"
	"sort"

	"github.com/bcfmtolgahan/lookout-operator/pkg/registry"
)

// LatestPolicy selects the most recently pushed image
type LatestPolicy struct {
	cfg        *Config
	tagPattern *regexp.Regexp
}

// NewLatestPolicy creates a new latest policy
func NewLatestPolicy(cfg *Config) *LatestPolicy {
	p := &LatestPolicy{cfg: cfg}

	// Compile tag pattern
	if cfg.TagPattern != "" {
		if re, err := regexp.Compile(cfg.TagPattern); err == nil {
			p.tagPattern = re
		}
	}

	return p
}

// Name returns the policy name
func (p *LatestPolicy) Name() string {
	return "latest"
}

// Select chooses the most recently pushed image
func (p *LatestPolicy) Select(candidates []registry.Image) *registry.Image {
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

// Filter filters images based on tag pattern and exclude/include lists
func (p *LatestPolicy) Filter(images []registry.Image) []registry.Image {
	var filtered []registry.Image

	for _, img := range images {
		// Check tag pattern
		if p.tagPattern != nil && !p.tagPattern.MatchString(img.Tag) {
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
