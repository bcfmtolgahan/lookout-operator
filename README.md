# Lookout

**The Kubernetes Image Watcher** - Automatically updates container images in your Kubernetes workloads.

[![Go Version](https://img.shields.io/badge/go-1.25-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## Overview

Lookout is a Kubernetes operator that watches container registries for new images and automatically updates your Deployments, DaemonSets, and StatefulSets.

### Features

- **Multi-Registry Support**: ECR, Harbor, DockerHub, GCR, ACR, and generic OCI registries
- **Annotation-Driven**: Configure updates per workload using annotations
- **Flexible Policies**: Semver, latest, digest, or regex-based image selection
- **Safety First**: Maintenance windows, approval workflows, automatic rollbacks
- **Observable**: Prometheus metrics, Kubernetes events, Slack notifications

## Quick Start

### 1. Install the Operator

```bash
# One-line installation (recommended)
kubectl apply -f https://raw.githubusercontent.com/bcfmtolgahan/lookout-operator/v1.0.0/install.yaml

# Or build from source
make install
make deploy IMG=ghcr.io/bcfmtolgahan/lookout-operator:v1.0.0
```

### 2. Create an ImageRegistry

```yaml
apiVersion: registry.lookout.dev/v1alpha1
kind: ImageRegistry
metadata:
  name: ecr-production
  namespace: lookout-system
spec:
  type: ecr  # must be lowercase
  ecr:
    region: eu-west-1
    accountId: "123456789012"  # Note: accountId not accountID
  auth:
    workloadIdentity:
      enabled: true  # Uses IRSA for authentication
```

> **Note**: For IRSA authentication, ensure your ServiceAccount has the `eks.amazonaws.com/role-arn` annotation pointing to an IAM role with ECR read permissions.

### 3. Annotate Your Workloads

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
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
          image: 123456789.dkr.ecr.eu-west-1.amazonaws.com/my-app:v1.0.0
```

## Annotations Reference

### Core

| Annotation | Description | Default |
|------------|-------------|---------|
| `lookout.dev/enabled` | Enable Lookout for this workload | - |
| `lookout.dev/registry` | ImageRegistry reference (name or namespace/name) | - |

### Policy

| Annotation | Description | Default |
|------------|-------------|---------|
| `lookout.dev/policy` | Policy type: `semver`, `latest`, `digest`, `regex` | `semver` |
| `lookout.dev/semver-range` | Semver constraint (e.g., `>=1.0.0`) | `*` |
| `lookout.dev/tag-pattern` | Regex pattern for tag filtering | `.*` |
| `lookout.dev/exclude-tags` | Tags to exclude (comma-separated) | - |

### Strategy

| Annotation | Description | Default |
|------------|-------------|---------|
| `lookout.dev/strategy` | Image format: `tag`, `digest`, `tag+digest` | `tag+digest` |
| `lookout.dev/interval` | Check interval | `5m` |

### Safety

| Annotation | Description | Default |
|------------|-------------|---------|
| `lookout.dev/suspend` | Temporarily stop updates | `false` |
| `lookout.dev/approval-required` | Require manual approval | `false` |
| `lookout.dev/auto-rollback` | Auto rollback on failure | `false` |
| `lookout.dev/maintenance-window` | Allowed update window (e.g., `02:00-06:00`) | - |
| `lookout.dev/maintenance-days` | Allowed days (e.g., `Sat,Sun`) | `*` |

### Notifications

| Annotation | Description |
|------------|-------------|
| `lookout.dev/notify-slack` | Slack channel or webhook URL |
| `lookout.dev/notify-webhook` | Custom webhook URL |

## Multi-Container Support

Configure different policies per container:

```yaml
annotations:
  lookout.dev/enabled: "true"

  # Container: app
  lookout.dev/container.app.enabled: "true"
  lookout.dev/container.app.policy: "semver"

  # Container: sidecar
  lookout.dev/container.sidecar.enabled: "true"
  lookout.dev/container.sidecar.policy: "digest"
```

## Supported Registries

| Registry | Type | Auth Methods |
|----------|------|--------------|
| AWS ECR | `ecr` | IRSA, Access Key |
| Harbor | `harbor` | Username/Password, Robot Account |
| DockerHub | `dockerhub` | Username/Token |
| Google GCR | `gcr` | Workload Identity, Service Account |
| Azure ACR | `acr` | Workload Identity, Service Principal |
| Generic OCI | `generic` | Basic Auth, Token |

## Metrics

| Metric | Description |
|--------|-------------|
| `lookout_registry_scan_duration_seconds` | Registry scan duration |
| `lookout_image_updates_total` | Total image updates |
| `lookout_rollbacks_total` | Total rollbacks |
| `lookout_workloads_watched` | Workloads being watched |
| `lookout_registry_circuit_state` | Circuit breaker state |

## Development

### Prerequisites
- Go 1.24+
- Docker 17.03+
- kubectl v1.11.3+
- Access to a Kubernetes cluster

### Building

```bash
# Generate code
make generate

# Generate manifests (CRDs, RBAC)
make manifests

# Build
make build

# Run tests
make test

# Run locally
make run
```

### Deploying

```bash
# Build and push image
make docker-build docker-push IMG=<registry>/lookout:tag

# Install CRDs
make install

# Deploy operator
make deploy IMG=<registry>/lookout:tag
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Lookout Operator                         │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────┐  │
│  │ Registry        │  │ Workload        │  │ Scanner     │  │
│  │ Controller      │  │ Controller      │  │ Controller  │  │
│  └─────────────────┘  └─────────────────┘  └─────────────┘  │
│           │                    │                  │         │
│           ▼                    ▼                  ▼         │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              Shared Image Cache                     │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
         │                       │                    │
         ▼                       ▼                    ▼
   ┌──────────┐           ┌──────────┐         ┌──────────┐
   │   ECR    │           │  Harbor  │         │ DockerHub│
   └──────────┘           └──────────┘         └──────────┘
```

## License

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
