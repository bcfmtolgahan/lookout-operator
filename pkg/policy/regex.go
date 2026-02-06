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

// RegexPolicy selects images based on regex pattern matching
type RegexPolicy struct {
	cfg     *Config
	pattern *regexp.Regexp
}

// NewRegexPolicy creates a new regex policy
func NewRegexPolicy(cfg *Config) *RegexPolicy {
	p := &RegexPolicy{cfg: cfg}

	// Compile pattern
	if cfg.TagPattern != "" {
		if re, err := regexp.Compile(cfg.TagPattern); err == nil {
			p.pattern = re
		}
	}

	return p
}

// Name returns the policy name
func (p *RegexPolicy) Name() string {
	return "regex"
}

// Select chooses the first matching image (sorted by push time)
func (p *RegexPolicy) Select(candidates []registry.Image) *registry.Image {
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

// Filter filters images based on regex pattern
func (p *RegexPolicy) Filter(images []registry.Image) []registry.Image {
	if p.pattern == nil {
		return images
	}

	var filtered []registry.Image

	for _, img := range images {
		// Check pattern
		if !p.pattern.MatchString(img.Tag) {
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

		filtered = append(filtered, img)
	}

	return filtered
}
