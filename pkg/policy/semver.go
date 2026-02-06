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
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/bcfmtolgahan/lookout-operator/pkg/registry"
)

// SemverPolicy selects images based on semantic versioning
type SemverPolicy struct {
	cfg        *Config
	constraint *semver.Constraints
	tagPattern *regexp.Regexp
}

// NewSemverPolicy creates a new semver policy
func NewSemverPolicy(cfg *Config) *SemverPolicy {
	p := &SemverPolicy{cfg: cfg}

	// Parse semver constraint
	if cfg.SemverRange != "" {
		if c, err := semver.NewConstraint(cfg.SemverRange); err == nil {
			p.constraint = c
		}
	}

	// Compile tag pattern
	if cfg.TagPattern != "" {
		if re, err := regexp.Compile(cfg.TagPattern); err == nil {
			p.tagPattern = re
		}
	}

	return p
}

// Name returns the policy name
func (p *SemverPolicy) Name() string {
	return "semver"
}

// Select chooses the best (highest) semver image
func (p *SemverPolicy) Select(candidates []registry.Image) *registry.Image {
	filtered := p.Filter(candidates)
	if len(filtered) == 0 {
		return nil
	}

	// Parse and sort by semver
	type versionedImage struct {
		image   registry.Image
		version *semver.Version
	}

	var versioned []versionedImage
	for _, img := range filtered {
		v := p.parseVersion(img.Tag)
		if v != nil {
			versioned = append(versioned, versionedImage{
				image:   img,
				version: v,
			})
		}
	}

	if len(versioned) == 0 {
		return nil
	}

	// Sort descending (highest version first)
	sort.Slice(versioned, func(i, j int) bool {
		return versioned[i].version.GreaterThan(versioned[j].version)
	})

	return &versioned[0].image
}

// Filter filters images based on semver rules
func (p *SemverPolicy) Filter(images []registry.Image) []registry.Image {
	var filtered []registry.Image

	for _, img := range images {
		// Check tag pattern
		if !p.matchesTagPattern(img.Tag) {
			continue
		}

		// Check exclude tags
		if p.isExcluded(img.Tag) {
			continue
		}

		// Check include tags (if specified)
		if len(p.cfg.IncludeTags) > 0 && !p.isIncluded(img.Tag) {
			continue
		}

		// Parse and check semver constraint
		v := p.parseVersion(img.Tag)
		if v == nil {
			continue
		}

		if p.constraint != nil && !p.constraint.Check(v) {
			continue
		}

		filtered = append(filtered, img)
	}

	return filtered
}

// parseVersion extracts semver from a tag
func (p *SemverPolicy) parseVersion(tag string) *semver.Version {
	// Remove common prefixes
	cleaned := tag
	for _, prefix := range []string{"v", "V", "version-", "release-"} {
		if strings.HasPrefix(cleaned, prefix) {
			cleaned = cleaned[len(prefix):]
			break
		}
	}

	v, err := semver.NewVersion(cleaned)
	if err != nil {
		// Try the original tag
		v, err = semver.NewVersion(tag)
		if err != nil {
			return nil
		}
	}

	return v
}

// matchesTagPattern checks if tag matches the pattern
func (p *SemverPolicy) matchesTagPattern(tag string) bool {
	if p.tagPattern == nil {
		return true
	}
	return p.tagPattern.MatchString(tag)
}

// isExcluded checks if tag is in exclude list
func (p *SemverPolicy) isExcluded(tag string) bool {
	for _, pattern := range p.cfg.ExcludeTags {
		if matchGlob(pattern, tag) {
			return true
		}
	}
	return false
}

// isIncluded checks if tag is in include list
func (p *SemverPolicy) isIncluded(tag string) bool {
	for _, pattern := range p.cfg.IncludeTags {
		if matchGlob(pattern, tag) {
			return true
		}
	}
	return false
}

// matchGlob performs simple glob matching
func matchGlob(pattern, s string) bool {
	// Handle exact match
	if pattern == s {
		return true
	}

	// Handle simple wildcard patterns
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		// *contains*
		return strings.Contains(s, pattern[1:len(pattern)-1])
	}
	if strings.HasPrefix(pattern, "*") {
		// *suffix
		return strings.HasSuffix(s, pattern[1:])
	}
	if strings.HasSuffix(pattern, "*") {
		// prefix*
		return strings.HasPrefix(s, pattern[:len(pattern)-1])
	}

	return false
}
