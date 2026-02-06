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

package annotations

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// WorkloadConfig holds parsed configuration for a workload
type WorkloadConfig struct {
	// Enabled indicates if Lookout is enabled for this workload
	Enabled bool

	// Registry is the ImageRegistry reference (namespace/name or just name)
	Registry string

	// RegistryNamespace is the parsed namespace from Registry
	RegistryNamespace string

	// RegistryName is the parsed name from Registry
	RegistryName string

	// Containers holds per-container configuration
	Containers map[string]*ContainerConfig

	// Default configuration (applies to all containers without specific config)
	Default *ContainerConfig

	// Safety settings
	Suspend          bool
	ApprovalRequired bool
	ApprovedBy       string
	ApprovedImage    string

	// Maintenance window
	MaintenanceWindow   string
	MaintenanceDays     []string
	MaintenanceTimezone string

	// Rollback settings
	AutoRollback    bool
	RollbackTimeout time.Duration
	RollbackTo      string
	PreviousImage   string

	// Notifications
	NotifySlack   string
	NotifyWebhook string
}

// ContainerConfig holds configuration for a specific container
type ContainerConfig struct {
	// Enabled indicates if this container should be updated
	Enabled bool

	// Registry override for this container
	Registry          string
	RegistryNamespace string
	RegistryName      string

	// Repository override (just the repo name, not full path)
	Repository string

	// Policy settings
	Policy      string
	SemverRange string
	TagPattern  string
	ExcludeTags []string
	IncludeTags []string

	// Strategy settings
	Strategy string
	Interval time.Duration
}

// Parser parses workload annotations
type Parser struct {
	containerPattern *regexp.Regexp
	initContainerPattern *regexp.Regexp
}

// NewParser creates a new annotation parser
func NewParser() *Parser {
	return &Parser{
		// Match: lookout.dev/container.<name>.<setting>
		containerPattern: regexp.MustCompile(`^` + regexp.QuoteMeta(Prefix) + `container\.([^.]+)\.(.+)$`),
		// Match: lookout.dev/initContainer.<name>.<setting>
		initContainerPattern: regexp.MustCompile(`^` + regexp.QuoteMeta(Prefix) + `initContainer\.([^.]+)\.(.+)$`),
	}
}

// Parse parses annotations into WorkloadConfig
func (p *Parser) Parse(annotations map[string]string) (*WorkloadConfig, error) {
	cfg := &WorkloadConfig{
		Containers: make(map[string]*ContainerConfig),
		Default:    &ContainerConfig{},
	}

	// Check if enabled
	if v, ok := annotations[Enabled]; ok {
		cfg.Enabled = parseBool(v)
	}

	if !cfg.Enabled {
		return cfg, nil
	}

	// Parse registry reference
	if v, ok := annotations[Registry]; ok {
		cfg.Registry = v
		cfg.RegistryNamespace, cfg.RegistryName = parseRegistryRef(v)
	}

	// Parse default policy settings
	p.parseContainerConfig(annotations, "", cfg.Default)

	// Parse container-specific settings
	for key, value := range annotations {
		// Check container pattern
		if matches := p.containerPattern.FindStringSubmatch(key); matches != nil {
			containerName := matches[1]
			setting := matches[2]

			if _, ok := cfg.Containers[containerName]; !ok {
				cfg.Containers[containerName] = &ContainerConfig{}
				// Copy defaults
				*cfg.Containers[containerName] = *cfg.Default
			}

			p.parseContainerSetting(cfg.Containers[containerName], setting, value)
		}

		// Check init container pattern
		if matches := p.initContainerPattern.FindStringSubmatch(key); matches != nil {
			containerName := "init:" + matches[1]
			setting := matches[2]

			if _, ok := cfg.Containers[containerName]; !ok {
				cfg.Containers[containerName] = &ContainerConfig{}
				*cfg.Containers[containerName] = *cfg.Default
			}

			p.parseContainerSetting(cfg.Containers[containerName], setting, value)
		}
	}

	// Parse safety settings
	if v, ok := annotations[Suspend]; ok {
		cfg.Suspend = parseBool(v)
	}
	if v, ok := annotations[ApprovalRequired]; ok {
		cfg.ApprovalRequired = parseBool(v)
	}
	if v, ok := annotations[ApprovedBy]; ok {
		cfg.ApprovedBy = v
	}
	if v, ok := annotations[ApprovedImage]; ok {
		cfg.ApprovedImage = v
	}

	// Parse maintenance window
	if v, ok := annotations[MaintenanceWindow]; ok {
		cfg.MaintenanceWindow = v
	}
	if v, ok := annotations[MaintenanceDays]; ok {
		cfg.MaintenanceDays = parseList(v)
	}
	if v, ok := annotations[MaintenanceTimezone]; ok {
		cfg.MaintenanceTimezone = v
	}

	// Parse rollback settings
	if v, ok := annotations[AutoRollback]; ok {
		cfg.AutoRollback = parseBool(v)
	}
	if v, ok := annotations[RollbackTimeout]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.RollbackTimeout = d
		}
	}
	if v, ok := annotations[RollbackTo]; ok {
		cfg.RollbackTo = v
	}
	if v, ok := annotations[PreviousImage]; ok {
		cfg.PreviousImage = v
	}

	// Parse notifications
	if v, ok := annotations[NotifySlack]; ok {
		cfg.NotifySlack = v
	}
	if v, ok := annotations[NotifyWebhook]; ok {
		cfg.NotifyWebhook = v
	}

	return cfg, nil
}

// parseContainerConfig parses default container configuration
func (p *Parser) parseContainerConfig(annotations map[string]string, prefix string, cfg *ContainerConfig) {
	cfg.Enabled = true

	if v, ok := annotations[Policy]; ok {
		cfg.Policy = v
	} else {
		cfg.Policy = DefaultPolicy
	}

	if v, ok := annotations[SemverRange]; ok {
		cfg.SemverRange = v
	}

	if v, ok := annotations[TagPattern]; ok {
		cfg.TagPattern = v
	}

	if v, ok := annotations[ExcludeTags]; ok {
		cfg.ExcludeTags = parseList(v)
	}

	if v, ok := annotations[IncludeTags]; ok {
		cfg.IncludeTags = parseList(v)
	}

	if v, ok := annotations[Strategy]; ok {
		cfg.Strategy = v
	} else {
		cfg.Strategy = DefaultStrategy
	}

	if v, ok := annotations[Interval]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Interval = d
		}
	} else {
		cfg.Interval, _ = time.ParseDuration(DefaultInterval)
	}
}

// parseContainerSetting parses a single container setting
func (p *Parser) parseContainerSetting(cfg *ContainerConfig, setting, value string) {
	switch setting {
	case ContainerEnabled:
		cfg.Enabled = parseBool(value)
	case ContainerRegistry:
		cfg.Registry = value
		cfg.RegistryNamespace, cfg.RegistryName = parseRegistryRef(value)
	case ContainerRepository:
		cfg.Repository = value
	case ContainerPolicy:
		cfg.Policy = value
	case ContainerSemverRange:
		cfg.SemverRange = value
	case ContainerTagPattern:
		cfg.TagPattern = value
	case ContainerExcludeTags:
		cfg.ExcludeTags = parseList(value)
	case ContainerIncludeTags:
		cfg.IncludeTags = parseList(value)
	case ContainerStrategy:
		cfg.Strategy = value
	case "interval":
		if d, err := time.ParseDuration(value); err == nil {
			cfg.Interval = d
		}
	}
}

// GetContainerConfig returns config for a specific container
// Falls back to default if no specific config exists
func (cfg *WorkloadConfig) GetContainerConfig(containerName string, isInit bool) *ContainerConfig {
	key := containerName
	if isInit {
		key = "init:" + containerName
	}

	if c, ok := cfg.Containers[key]; ok {
		return c
	}

	return cfg.Default
}

// IsApproved checks if an update is approved
func (cfg *WorkloadConfig) IsApproved(targetImage string) bool {
	if !cfg.ApprovalRequired {
		return true
	}

	if cfg.ApprovedBy == "" {
		return false
	}

	// If specific image is approved, check it matches
	if cfg.ApprovedImage != "" {
		return cfg.ApprovedImage == targetImage || strings.HasSuffix(targetImage, ":"+cfg.ApprovedImage)
	}

	return true
}

// Validate validates the configuration
func (cfg *WorkloadConfig) Validate() error {
	if !cfg.Enabled {
		return nil
	}

	if cfg.Registry == "" && cfg.Default.Registry == "" {
		// Check if all containers have registry
		for name, c := range cfg.Containers {
			if c.Enabled && c.Registry == "" {
				return fmt.Errorf("container %s has no registry specified", name)
			}
		}
	}

	// Validate policy
	validPolicies := map[string]bool{
		PolicySemver: true,
		PolicyLatest: true,
		PolicyDigest: true,
		PolicyRegex:  true,
	}

	if cfg.Default.Policy != "" && !validPolicies[cfg.Default.Policy] {
		return fmt.Errorf("invalid policy: %s", cfg.Default.Policy)
	}

	for name, c := range cfg.Containers {
		if c.Policy != "" && !validPolicies[c.Policy] {
			return fmt.Errorf("container %s has invalid policy: %s", name, c.Policy)
		}
	}

	// Validate strategy
	validStrategies := map[string]bool{
		StrategyTag:          true,
		StrategyDigest:       true,
		StrategyTagAndDigest: true,
	}

	if cfg.Default.Strategy != "" && !validStrategies[cfg.Default.Strategy] {
		return fmt.Errorf("invalid strategy: %s", cfg.Default.Strategy)
	}

	return nil
}

// Helper functions

func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

func parseList(s string) []string {
	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func parseRegistryRef(ref string) (namespace, name string) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}

// DefaultParser is the default annotation parser
var DefaultParser = NewParser()

// Parse parses annotations using the default parser
func Parse(annotations map[string]string) (*WorkloadConfig, error) {
	return DefaultParser.Parse(annotations)
}
