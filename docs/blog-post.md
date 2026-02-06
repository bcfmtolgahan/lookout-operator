# Building Lookout: A Kubernetes Operator for Automated Image Updates

Managing container image updates across dozens of microservices is tedious. Every time a new image is pushed to your registry, someone has to update the deployment manifest, commit it, and wait for the rollout. This post walks through the design and implementation of Lookout Operator - a Kubernetes operator that automates this process.

## The Problem

Consider a typical microservices architecture with 50+ services running on Kubernetes. Each service has its own CI pipeline that builds and pushes images to ECR. The deployment flow looks like this:

```
CI builds image → Push to ECR → Manual manifest update → Git commit → ArgoCD sync → Deployment
```

The manual step in the middle creates friction:
- Security patches are delayed because someone needs to update manifests
- Teams spend time on repetitive YAML changes
- Inconsistent update practices across services

## The Solution

Lookout Operator watches your container registry and automatically updates workloads when new images match your policy:

```
CI builds image → Push to ECR → Lookout detects → Policy check → Auto-update Deployment
```

No manifest changes. No commits. No manual intervention.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                      Lookout Operator                           │
│                                                                 │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────┐  │
│  │    Registry      │  │    Workload      │  │   Policy     │  │
│  │   Controller     │  │   Controller     │  │   Engine     │  │
│  └────────┬─────────┘  └────────┬─────────┘  └──────┬───────┘  │
│           │                     │                    │          │
│           ▼                     ▼                    ▼          │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    Registry Factory                      │   │
│  │         (ECR, Harbor, DockerHub, GCR, ACR)              │   │
│  └─────────────────────────────────────────────────────────┘   │
│           │                                                     │
│           ▼                                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │    Circuit Breaker    │    Rate Limiter    │    Cache   │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │   AWS ECR API    │
                    └──────────────────┘
```

### Core Components

**1. ImageRegistry CRD** - Defines registry connections (ECR, Harbor, etc.)

**2. Registry Controller** - Reconciles ImageRegistry resources, validates connections

**3. Workload Controller** - Watches annotated Deployments/DaemonSets/StatefulSets, triggers updates

**4. Policy Engine** - Determines which image to select (semver, latest, regex, digest)

**5. Registry Factory** - Creates registry clients based on type

## Deep Dive: The Custom Resource Definition

The ImageRegistry CRD is the foundation. Here's the Go struct:

```go
type ImageRegistrySpec struct {
    // Type of registry: ecr, harbor, dockerhub, gcr, acr, generic
    Type RegistryType `json:"type"`

    // ECR-specific configuration
    ECR *ECRConfig `json:"ecr,omitempty"`

    // Authentication configuration
    Auth *AuthConfig `json:"auth,omitempty"`

    // Health check interval
    CheckInterval metav1.Duration `json:"checkInterval,omitempty"`
}

type ECRConfig struct {
    Region    string `json:"region"`
    AccountID string `json:"accountId"`
}
```

Key design decision: **registry configuration is separate from workload configuration**. This allows multiple workloads to share the same registry connection while having different update policies.

## Deep Dive: Annotation-Based Configuration

Instead of creating a CRD for each workload (like Flux Image Policy), Lookout uses annotations:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-service
  annotations:
    lookout.dev/enabled: "true"
    lookout.dev/registry: "lookout-system/ecr-production"
    lookout.dev/policy: "semver"
    lookout.dev/semver-range: ">=1.0.0"
spec:
  template:
    spec:
      containers:
        - name: app
          image: 123456789.dkr.ecr.eu-west-1.amazonaws.com/my-service:v1.0.0
```

The annotation parser extracts configuration:

```go
type Config struct {
    Enabled          bool
    Registry         string
    Policy           string
    SemverRange      string
    TagPattern       string
    ExcludeTags      []string
    Interval         time.Duration
    ApprovalRequired bool
    Suspended        bool
    // ... more fields
}

func Parse(annotations map[string]string) (*Config, error) {
    cfg := &Config{}

    if v, ok := annotations[Prefix+"enabled"]; ok {
        cfg.Enabled = v == "true"
    }

    if v, ok := annotations[Prefix+"registry"]; ok {
        cfg.Registry = v
    }

    // ... parse other annotations

    return cfg, nil
}
```

Why annotations over CRDs?
- **Simpler UX** - No extra resources to create/manage
- **Portable** - Annotations travel with the workload
- **Familiar** - Similar to how Prometheus scraping works

## Deep Dive: The Policy Engine

The policy engine determines which image to select from available tags. Four policies are supported:

### 1. Semver Policy

Selects the highest version matching a semver constraint:

```go
type SemverPolicy struct {
    cfg        *Config
    constraint *semver.Constraints
}

func (p *SemverPolicy) Select(candidates []registry.Image) *registry.Image {
    var matching []registry.Image

    for _, img := range candidates {
        // Parse tag as semver
        v, err := semver.NewVersion(img.Tag)
        if err != nil {
            continue // Skip non-semver tags
        }

        // Check constraint
        if p.constraint.Check(v) {
            matching = append(matching, img)
        }
    }

    // Sort by version descending
    sort.Slice(matching, func(i, j int) bool {
        vi, _ := semver.NewVersion(matching[i].Tag)
        vj, _ := semver.NewVersion(matching[j].Tag)
        return vi.GreaterThan(vj)
    })

    if len(matching) == 0 {
        return nil
    }
    return &matching[0]
}
```

Supported constraints:
- `>=1.0.0` - Version 1.0.0 or higher
- `^1.2.0` - Compatible with 1.2.0 (same major version)
- `~1.2.0` - Approximately 1.2.0 (same major.minor)
- `1.0.0 - 2.0.0` - Range between versions

### 2. Latest Policy

Selects the most recently pushed image:

```go
func (p *LatestPolicy) Select(candidates []registry.Image) *registry.Image {
    filtered := p.Filter(candidates)

    sort.Slice(filtered, func(i, j int) bool {
        return filtered[i].PushedAt.After(filtered[j].PushedAt)
    })

    return &filtered[0]
}
```

### 3. Regex Policy

Selects images matching a regex pattern:

```go
func (p *RegexPolicy) Filter(images []registry.Image) []registry.Image {
    var filtered []registry.Image

    for _, img := range images {
        if p.pattern.MatchString(img.Tag) {
            filtered = append(filtered, img)
        }
    }

    return filtered
}
```

### 4. Digest Policy

Pins to a specific image digest (no automatic updates, used for explicit control).

## Deep Dive: ECR Integration

The ECR client uses AWS SDK v2 with support for IRSA (IAM Roles for Service Accounts):

```go
func NewECRRegistry(ctx context.Context, cfg *registry.Config) (*ECRRegistry, error) {
    // Load AWS config - automatically uses IRSA if available
    awsCfg, err := config.LoadDefaultConfig(ctx,
        config.WithRegion(cfg.Region),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to load AWS config: %w", err)
    }

    return &ECRRegistry{
        client:    ecr.NewFromConfig(awsCfg),
        accountID: cfg.AccountID,
        region:    cfg.Region,
    }, nil
}
```

Listing images from ECR:

```go
func (r *ECRRegistry) ListImages(ctx context.Context, repository string) ([]registry.Image, error) {
    var images []registry.Image

    paginator := ecr.NewDescribeImagesPaginator(r.client, &ecr.DescribeImagesInput{
        RepositoryName: aws.String(repository),
        RegistryId:     aws.String(r.accountID),
    })

    for paginator.HasMorePages() {
        page, err := paginator.NextPage(ctx)
        if err != nil {
            return nil, err
        }

        for _, img := range page.ImageDetails {
            for _, tag := range img.ImageTags {
                images = append(images, registry.Image{
                    Repository: repository,
                    Tag:        tag,
                    Digest:     aws.ToString(img.ImageDigest),
                    PushedAt:   aws.ToTime(img.ImagePushedAt),
                })
            }
        }
    }

    return images, nil
}
```

## Deep Dive: Circuit Breaker

Registry APIs can be flaky. A circuit breaker prevents cascading failures:

```go
type CircuitBreaker struct {
    mu              sync.RWMutex
    state           State
    failures        int
    successes       int
    failureThreshold int
    successThreshold int
    timeout         time.Duration
    lastFailure     time.Time
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
    if !cb.AllowRequest() {
        return ErrCircuitOpen
    }

    err := fn()

    if err != nil {
        cb.RecordFailure()
        return err
    }

    cb.RecordSuccess()
    return nil
}

func (cb *CircuitBreaker) AllowRequest() bool {
    cb.mu.RLock()
    defer cb.mu.RUnlock()

    switch cb.state {
    case StateClosed:
        return true
    case StateOpen:
        // Check if timeout has passed
        if time.Since(cb.lastFailure) > cb.timeout {
            cb.mu.RUnlock()
            cb.mu.Lock()
            cb.state = StateHalfOpen
            cb.mu.Unlock()
            cb.mu.RLock()
            return true
        }
        return false
    case StateHalfOpen:
        return true
    }
    return false
}
```

States:
- **Closed** - Normal operation, requests pass through
- **Open** - Too many failures, requests are blocked
- **Half-Open** - Testing if service recovered

## Deep Dive: Rate Limiting

To avoid hitting registry API limits:

```go
type RateLimiter struct {
    limiters map[string]*rate.Limiter
    mu       sync.RWMutex
    rate     rate.Limit
    burst    int
}

func (r *RateLimiter) Wait(ctx context.Context, key string) error {
    limiter := r.getLimiter(key)
    return limiter.Wait(ctx)
}

func (r *RateLimiter) getLimiter(key string) *rate.Limiter {
    r.mu.RLock()
    limiter, exists := r.limiters[key]
    r.mu.RUnlock()

    if exists {
        return limiter
    }

    r.mu.Lock()
    defer r.mu.Unlock()

    limiter = rate.NewLimiter(r.rate, r.burst)
    r.limiters[key] = limiter
    return limiter
}
```

Default: 10 requests per second per registry.

## Deep Dive: The Reconciliation Loop

The workload controller's reconciliation loop:

```go
func (r *WorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch the workload
    var deployment appsv1.Deployment
    if err := r.Get(ctx, req.NamespacedName, &deployment); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Parse annotations
    cfg, err := annotations.Parse(deployment.Annotations)
    if err != nil || !cfg.Enabled {
        return ctrl.Result{}, nil
    }

    // 3. Get registry client
    registry, err := r.Factory.Get(ctx, cfg.Registry)
    if err != nil {
        return ctrl.Result{}, err
    }

    // 4. Extract current image info
    currentImage := deployment.Spec.Template.Spec.Containers[0].Image
    repo, currentTag := parseImage(currentImage)

    // 5. List available images
    images, err := registry.ListImages(ctx, repo)
    if err != nil {
        return ctrl.Result{}, err
    }

    // 6. Apply policy
    policy, _ := r.PolicyEngine.Create(&policy.Config{
        PolicyType:  cfg.Policy,
        SemverRange: cfg.SemverRange,
    })

    selected := policy.Select(images)
    if selected == nil || selected.Tag == currentTag {
        // No update needed
        return ctrl.Result{RequeueAfter: cfg.Interval}, nil
    }

    // 7. Check approval requirement
    if cfg.ApprovalRequired {
        return r.handleApproval(ctx, &deployment, selected)
    }

    // 8. Update the deployment
    newImage := fmt.Sprintf("%s:%s@%s", repo, selected.Tag, selected.Digest)
    deployment.Spec.Template.Spec.Containers[0].Image = newImage

    if err := r.Update(ctx, &deployment); err != nil {
        return ctrl.Result{}, err
    }

    // 9. Record event
    r.Recorder.Event(&deployment, "Normal", "ImageUpdated",
        fmt.Sprintf("Updated image to %s", selected.Tag))

    return ctrl.Result{RequeueAfter: cfg.Interval}, nil
}
```

## Approval Workflow

For critical workloads, you might want manual approval before updates:

```yaml
annotations:
  lookout.dev/enabled: "true"
  lookout.dev/approval-required: "true"
```

When a new image is detected:

1. Lookout updates the status annotation with available update
2. Operator waits for approval annotation
3. User approves by setting the annotation
4. Lookout proceeds with update

```yaml
# Status set by Lookout
annotations:
  lookout.dev/status.available-update: "v1.2.0"
  lookout.dev/status.current-image: "v1.1.0"

# User approves
annotations:
  lookout.dev/approve: "v1.2.0"
```

## Maintenance Windows

Restrict updates to specific time windows:

```yaml
annotations:
  lookout.dev/maintenance-window: "02:00-06:00"
  lookout.dev/maintenance-days: "Sat,Sun"
```

Implementation:

```go
func isInMaintenanceWindow(window, days string) bool {
    now := time.Now()

    // Check day
    if days != "*" {
        allowedDays := strings.Split(days, ",")
        currentDay := now.Weekday().String()[:3]
        if !contains(allowedDays, currentDay) {
            return false
        }
    }

    // Check time window
    parts := strings.Split(window, "-")
    start, _ := time.Parse("15:04", parts[0])
    end, _ := time.Parse("15:04", parts[1])

    currentTime := now.Hour()*60 + now.Minute()
    startMinutes := start.Hour()*60 + start.Minute()
    endMinutes := end.Hour()*60 + end.Minute()

    return currentTime >= startMinutes && currentTime <= endMinutes
}
```

## Installation

One-line installation:

```bash
kubectl apply -f https://raw.githubusercontent.com/bcfmtolgahan/lookout-operator/v1.0.0/install.yaml
```

This installs:
- Namespace `lookout-system`
- ImageRegistry CRD
- RBAC (ClusterRole, ServiceAccount, ClusterRoleBinding)
- Deployment with the operator

For ECR with IRSA, annotate the ServiceAccount:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: lookout-controller-manager
  namespace: lookout-system
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/LookoutOperatorRole
```

## Performance Considerations

**Caching**: Registry responses are cached to reduce API calls. Default TTL is 5 minutes.

**Batch Processing**: The controller processes workloads in batches to avoid overwhelming the API server.

**Leader Election**: Only one replica performs reconciliation to prevent conflicts.

**Exponential Backoff**: Failed reconciliations use exponential backoff to prevent tight loops.

## Future Improvements

1. **Prometheus Metrics** - Expose circuit breaker state, update counts, scan duration
2. **Multi-container Support** - Update specific containers in multi-container pods
3. **Webhook Notifications** - Slack, Teams, PagerDuty integration
4. **Rollback Detection** - Detect failed deployments and auto-rollback

## Conclusion

Lookout Operator removes the manual step from the image update workflow. By using annotation-based configuration and pluggable policies, it provides flexibility without complexity.

The code is open source: [github.com/bcfmtolgahan/lookout-operator](https://github.com/bcfmtolgahan/lookout-operator)

---

*Built with Go, Kubebuilder, and controller-runtime.*
