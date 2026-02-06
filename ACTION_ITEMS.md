# Lookout Operator - Action Items

## Bugs

### 1. Silent Failure on Wrong Version Approval
- **Severity**: Minor
- **Description**: When user approves a version different from `status.available-update`, the approval silently fails without any error or event
- **Expected**: Should emit an event or update status with error message
- **File**: `internal/controller/workload_controller.go`

## Feature TODOs

### 2. Prometheus Metrics for Circuit Breaker
- **Priority**: Medium
- **Description**: Expose circuit breaker state and statistics to Prometheus
- **Metrics to add**:
  - `lookout_circuit_breaker_state` (gauge: 0=closed, 1=half-open, 2=open)
  - `lookout_circuit_breaker_failures_total` (counter)
  - `lookout_circuit_breaker_successes_total` (counter)
- **File**: `pkg/circuitbreaker/circuitbreaker.go`

### 3. Rate Limiter Burst Configuration
- **Priority**: Low
- **Description**: Add configurable burst setting for rate limiter (currently hardcoded)
- **File**: `pkg/ratelimit/ratelimit.go`

### 4. Multi-Container Pod Support
- **Priority**: Medium
- **Description**: Support pods with multiple containers, allow specifying which container to update via annotation
- **Annotation**: `lookout.io/container=<container-name>`
- **File**: `pkg/workload/workload.go`

## Documentation

### 5. IRSA Setup Guide
- **Priority**: High
- **Description**: Add detailed guide for setting up IRSA (IAM Roles for Service Accounts) for ECR authentication
- **Location**: `docs/irsa-setup.md` or README section

### 6. Troubleshooting Guide
- **Priority**: Medium
- **Description**: Add common issues and solutions
- **Topics**:
  - Image pull errors
  - Permission issues
  - Registry connectivity
  - Policy matching failures
- **Location**: `docs/troubleshooting.md` or README section

### 7. CRD Field Case Sensitivity Note
- **Priority**: High
- **Description**: Document that CRD fields use camelCase (e.g., `accountId` not `accountID`, `type: ecr` not `type: ECR`)
- **Location**: README examples and API documentation

---

## Completed
- [x] Initial release v1.0.0
- [x] GitHub repository published
- [x] install.yaml generated
