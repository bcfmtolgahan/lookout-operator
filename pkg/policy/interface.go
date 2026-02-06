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
	"github.com/bcfmtolgahan/lookout-operator/pkg/registry"
)

// Policy defines how to select an image from available images
type Policy interface {
	// Name returns the policy name
	Name() string

	// Select chooses the best image from candidates based on policy rules
	// Returns nil if no image matches
	Select(candidates []registry.Image) *registry.Image

	// Filter filters images based on policy rules
	// Returns images that match the policy
	Filter(images []registry.Image) []registry.Image
}

// Config holds policy configuration
type Config struct {
	// PolicyType is the policy type (semver, latest, digest, regex)
	PolicyType string

	// SemverRange is the semver constraint (e.g., ">=1.0.0")
	SemverRange string

	// TagPattern is a regex pattern for filtering tags
	TagPattern string

	// ExcludeTags are tags to exclude (comma-separated or glob patterns)
	ExcludeTags []string

	// IncludeTags are tags to include (comma-separated or glob patterns)
	IncludeTags []string
}

// Engine creates and manages policies
type Engine struct {
	policies map[string]func(cfg *Config) Policy
}

// NewEngine creates a new policy engine
func NewEngine() *Engine {
	e := &Engine{
		policies: make(map[string]func(cfg *Config) Policy),
	}

	// Register built-in policies
	e.Register("semver", func(cfg *Config) Policy {
		return NewSemverPolicy(cfg)
	})
	e.Register("latest", func(cfg *Config) Policy {
		return NewLatestPolicy(cfg)
	})
	e.Register("digest", func(cfg *Config) Policy {
		return NewDigestPolicy(cfg)
	})
	e.Register("regex", func(cfg *Config) Policy {
		return NewRegexPolicy(cfg)
	})

	return e
}

// Register registers a policy factory
func (e *Engine) Register(name string, factory func(cfg *Config) Policy) {
	e.policies[name] = factory
}

// Create creates a policy from configuration
func (e *Engine) Create(cfg *Config) (Policy, error) {
	factory, ok := e.policies[cfg.PolicyType]
	if !ok {
		// Default to semver
		factory = e.policies["semver"]
	}

	return factory(cfg), nil
}

// DefaultEngine is the default policy engine
var DefaultEngine = NewEngine()

// Create creates a policy using the default engine
func Create(cfg *Config) (Policy, error) {
	return DefaultEngine.Create(cfg)
}
