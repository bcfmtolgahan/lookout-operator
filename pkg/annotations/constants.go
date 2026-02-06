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

// Annotation prefix
const (
	// Prefix is the annotation prefix for all Lookout annotations
	Prefix = "lookout.dev/"
)

// Core annotations
const (
	// Enabled enables Lookout for this workload
	// Value: "true" or "false"
	Enabled = Prefix + "enabled"

	// Registry references the ImageRegistry CRD name
	// Value: "registry-name" or "namespace/registry-name"
	Registry = Prefix + "registry"
)

// Container-level annotation patterns
const (
	// ContainerPrefix is the prefix for container-specific annotations
	// Pattern: lookout.dev/container.<name>.<setting>
	ContainerPrefix = Prefix + "container."

	// InitContainerPrefix is the prefix for init container annotations
	// Pattern: lookout.dev/initContainer.<name>.<setting>
	InitContainerPrefix = Prefix + "initContainer."
)

// Container settings (append to ContainerPrefix + containerName + ".")
const (
	ContainerEnabled    = "enabled"
	ContainerRegistry   = "registry"
	ContainerRepository = "repository"
	ContainerPolicy     = "policy"
	ContainerSemverRange = "semver-range"
	ContainerTagPattern = "tag-pattern"
	ContainerExcludeTags = "exclude-tags"
	ContainerIncludeTags = "include-tags"
	ContainerStrategy   = "strategy"
)

// Policy annotations
const (
	// Policy defines the tag selection policy
	// Values: "semver", "latest", "digest", "regex"
	Policy = Prefix + "policy"

	// SemverRange defines the semver constraint
	// Value: ">=1.0.0", "^2.0.0", etc.
	SemverRange = Prefix + "semver-range"

	// TagPattern defines a regex pattern for tag filtering
	// Value: "^v[0-9]+\\.[0-9]+\\.[0-9]+$"
	TagPattern = Prefix + "tag-pattern"

	// ExcludeTags defines tags to exclude
	// Value: "dev,test,latest,*-rc*"
	ExcludeTags = Prefix + "exclude-tags"

	// IncludeTags defines tags to include (whitelist)
	// Value: "v*,release-*"
	IncludeTags = Prefix + "include-tags"
)

// Update strategy annotations
const (
	// Strategy defines how to format the image reference
	// Values: "tag", "digest", "tag+digest"
	Strategy = Prefix + "strategy"

	// Interval defines the check interval
	// Value: "1m", "5m", "1h"
	Interval = Prefix + "interval"
)

// Safety and control annotations
const (
	// Suspend temporarily stops updates
	// Value: "true" or "false"
	Suspend = Prefix + "suspend"

	// ApprovalRequired requires manual approval before updates
	// Value: "true" or "false"
	ApprovalRequired = Prefix + "approval-required"

	// ApprovedBy is set when approval is granted
	// Value: "user@email.com"
	ApprovedBy = Prefix + "approved-by"

	// ApprovedImage is the specific image approved for update
	// Value: "v1.3.0"
	ApprovedImage = Prefix + "approved-image"

	// MaintenanceWindow defines when updates are allowed
	// Value: "02:00-06:00"
	MaintenanceWindow = Prefix + "maintenance-window"

	// MaintenanceDays defines which days updates are allowed
	// Value: "Mon,Tue,Wed,Thu,Fri" or "*"
	MaintenanceDays = Prefix + "maintenance-days"

	// MaintenanceTimezone defines the timezone for maintenance window
	// Value: "Europe/Istanbul", "UTC"
	MaintenanceTimezone = Prefix + "maintenance-tz"
)

// Rollback annotations
const (
	// AutoRollback enables automatic rollback on failure
	// Value: "true" or "false"
	AutoRollback = Prefix + "auto-rollback"

	// RollbackTimeout is the time to wait before rollback
	// Value: "5m"
	RollbackTimeout = Prefix + "rollback-timeout"

	// RollbackTo triggers a manual rollback
	// Value: "previous" or specific tag like "v1.2.0"
	RollbackTo = Prefix + "rollback-to"

	// PreviousImage stores the previous image (set by operator)
	// Value: "registry/image:tag@sha256:..."
	PreviousImage = Prefix + "previous-image"
)

// Notification annotations
const (
	// NotifySlack is the Slack channel or webhook URL
	// Value: "#channel" or "https://hooks.slack.com/..."
	NotifySlack = Prefix + "notify-slack"

	// NotifyWebhook is a custom webhook URL
	// Value: "https://..."
	NotifyWebhook = Prefix + "notify-webhook"
)

// Status annotations (read-only, set by operator)
const (
	// StatusPrefix is the prefix for status annotations
	StatusPrefix = Prefix + "status."

	// StatusLastCheck is the last check time
	StatusLastCheck = StatusPrefix + "last-check"

	// StatusLastUpdate is the last update time
	StatusLastUpdate = StatusPrefix + "last-update"

	// StatusCurrentDigest is the current image digest
	StatusCurrentDigest = StatusPrefix + "current-digest"

	// StatusAvailableUpdate is a pending update (when approval required)
	StatusAvailableUpdate = StatusPrefix + "available-update"

	// StatusUpdateCount is the total update count
	StatusUpdateCount = StatusPrefix + "update-count"

	// StatusError is the last error message
	StatusError = StatusPrefix + "error"
)

// Policy values
const (
	PolicySemver = "semver"
	PolicyLatest = "latest"
	PolicyDigest = "digest"
	PolicyRegex  = "regex"
)

// Strategy values
const (
	StrategyTag          = "tag"
	StrategyDigest       = "digest"
	StrategyTagAndDigest = "tag+digest"
)

// Default values
const (
	DefaultPolicy   = PolicySemver
	DefaultStrategy = StrategyTagAndDigest
	DefaultInterval = "5m"
)

// AllAnnotations returns all known annotation keys for validation
func AllAnnotations() []string {
	return []string{
		Enabled,
		Registry,
		Policy,
		SemverRange,
		TagPattern,
		ExcludeTags,
		IncludeTags,
		Strategy,
		Interval,
		Suspend,
		ApprovalRequired,
		ApprovedBy,
		ApprovedImage,
		MaintenanceWindow,
		MaintenanceDays,
		MaintenanceTimezone,
		AutoRollback,
		RollbackTimeout,
		RollbackTo,
		PreviousImage,
		NotifySlack,
		NotifyWebhook,
		StatusLastCheck,
		StatusLastUpdate,
		StatusCurrentDigest,
		StatusAvailableUpdate,
		StatusUpdateCount,
		StatusError,
	}
}
