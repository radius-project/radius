# Radius Control Plane Upgrades

- **Author**: Yetkin Timocin (@ytimocin)

## Overview

Radius is an open-source, cloud-native application platform that enables developers and operators to define, deploy, and collaborate on applications across cloud environments. As Radius evolves with new features and improvements, users need a reliable way to upgrade their installations without disruption or data loss.

This feature introduces in-place upgrades for the Radius Control Plane, allowing users to seamlessly update their Radius installations to newer versions without the current cumbersome process of uninstalling and reinstalling. By running a single command (`rad upgrade kubernetes`), users can update all Radius components while preserving their application deployments and configurations. **It's important to note that user applications deployed through Radius continue running without interruption during this process, as Radius only maintains deployment metadata and does not directly control application runtime execution.**

## Terms and definitions

| Term                     | Definition                                                                                                                                                                                                                                                                                                                                                                   |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Radius Control Plane** | The set of management components in Radius that coordinate application lifecycle operations. These include the Universal Control Plane, Deployment Engine, Applications Resource Provider, etc.                                                                                                                                                                              |
| **In-place Upgrade**     | An upgrade approach that updates an existing Radius installation in its current location without requiring complete reinstallation. The process preserves all user data, configurations, and deployed application metadata while minimizing downtime of the control plane. This contrasts with alternative approaches like parallel deployments or complete reinstallations. |
| **Rolling Upgrade**      | An upgrade strategy that updates components incrementally (one by one) rather than all at once, reducing downtime.                                                                                                                                                                                                                                                           |
| **User Data Backup**     | A point-in-time backup of user data taken before an upgrade to enable recovery if problems occur.                                                                                                                                                                                                                                                                            |
| **User Data Restore**    | The process of reverting user data to a previously backed up state after an upgrade attempt encounters issues.                                                                                                                                                                                                                                                               |
| **Pre-flight Checks**    | Validation steps performed before an upgrade to ensure prerequisites are met and the system is in a valid state for upgrading.                                                                                                                                                                                                                                               |
| **Version Skipping**     | The ability to upgrade directly from one version to a newer non-adjacent version (e.g., v0.40 → v0.44) without installing intermediate versions.                                                                                                                                                                                                                             |

## Assumptions

1. **User permissions**: Users running the upgrade command have enough permissions on both the Kubernetes cluster and the Radius installation.

2. **Resource requirements**: The Kubernetes cluster has sufficient compute resources (CPU, memory) to run both the existing and new version components during the rolling upgrade process.

3. **CLI version compatibility**: User has a Radius CLI version that includes the `rad upgrade kubernetes` feature. While older CLIs can't perform upgrades, newer CLIs maintain backward compatibility with older control planes.

   Example: If you have CLI v0.42 and Control Plane v0.42:

   - You CAN upgrade to Control Plane v0.44 using the v0.42 CLI (if v0.42 CLI includes the upgrade feature)
   - After upgrading, your CLI (v0.42) will still work with basic operations against your v0.44 Control Plane
   - However, you won't be able to use any new v0.44 features from your v0.42 CLI
   - For full functionality, you should upgrade your CLI to match the Control Plane version after the upgrade completes

4. **Network connectivity**: The upgrade process requires internet connectivity to pull container images from registries and validate versions. The process is not intended to run in air-gapped environments.

   - An air-gapped environment is one where systems are physically isolated from unsecured networks like the public internet.
   - These environments are common in high-security scenarios (military, financial, healthcare, government) where external network connectivity is restricted.

5. **Stable starting state**: The Radius installation being upgraded is in a healthy, stable state. Attempting to upgrade an already failing installation may lead to unpredictable results.

6. **Storage availability**: The Kubernetes cluster has sufficient persistent storage capacity for backup operations during the upgrade process.

## Objectives

> **Issue Reference:** <https://github.com/radius-project/radius/issues/8095>

### Goals

- **Simplify upgrade process**: Provide a single CLI command (`rad upgrade kubernetes`) to upgrade Radius without manual reinstallation.
- **Ensure data safety**: Implement automatic user data backups before upgrades and restore capability if failures occur.
- **Minimize downtime**: Use rolling upgrades where possible to keep Radius control plane available during the upgrade process.
- **Preserve application continuity**: Ensure that user applications continue running without interruption throughout the Radius upgrade process.

### Non goals

- **Downgrade support**: The upgrade process is designed to move forward to newer versions only. Downgrading to previous versions is not supported.
- **Multi-cluster upgrades**: Managing upgrades across multiple clusters simultaneously is out of scope, as Radius doesn't support multiple clusters per installation as of March 2025.
- **Dependency major version upgrades**: Upgrading major versions of dependencies like Postgres, Dapr, or Contour is not covered. These upgrades should be handled separately following their respective guidelines.
- **Full Helm upgrade support**: While we use Helm internally, making `helm upgrade` work completely for Radius is not in scope. Running `helm upgrade` on a Radius Helm installation doesn't configure all necessary components for the control plane to work properly.
- **Zero-downtime control plane**: While we aim to minimize disruption to the control plane itself, guaranteeing absolutely no downtime for the Radius control plane components is not a goal for this initial release.
- **Automatic CLI upgrades**: The upgrade command updates only the Radius control plane components running in Kubernetes. It does not automatically update your local Radius CLI version. You must manually download and install the matching CLI version separately.
- **Application management during upgrades**: Since Radius only maintains deployment metadata and doesn't control runtime execution of user applications, managing or modifying user workloads during the upgrade is explicitly not in scope.

### User scenarios (optional)

**Important Note:** Radius upgrades only affect the control plane components and deployment metadata. User applications deployed through Radius continue running without interruption, as Radius does not manage their runtime execution.

The primary users of this feature are system administrators, application developers, and DevOps engineers responsible for maintaining and upgrading the Radius platform.

- **System Administrator**:
  Responsible for managing the infrastructure and ensuring the smooth operation of the Radius platform. They have advanced knowledge of Kubernetes administration and are concerned with stability, security, and minimizing downtime during upgrades. They need clear visibility into the upgrade process and rollback options if issues occur.
- **DevOps Engineer**:
  Focused on automating deployment processes and maintaining the CI/CD pipeline. They typically integrate Radius into larger workflows and want predictable, scriptable upgrade paths that can be incorporated into their automation systems.
- **Application Developer**:
  Uses Radius to deploy and manage their applications but isn't deeply involved in platform administration. They primarily care that their applications continue running during Radius upgrades and that any API or interface changes are clearly documented.

#### Scenario 1: System Administrator performs a standard version upgrade

**User Story:** As a system administrator, I need to upgrade Radius to a newer version while maintaining continuous operation of user applications, as Radius only stores deployment metadata and doesn't control application runtime.

**User Experience:**

```bash
# Check current Radius version
> rad version
RELEASE   VERSION   BICEP     COMMIT
0.44.0    v0.44.0   0.33.93   1dd17270ec9bc9e764f314fa62c248406034edda

# Perform a basic upgrade to a specific version
> rad upgrade kubernetes --version v0.45.0

Initiating Radius upgrade from v0.44.0 to v0.45.0...
Pre-flight checks:
  ✓ Valid version target
  ✓ Compatible upgrade path
Upgrading control plane components:
  ✓ Universal Control Plane
  ✓ Deployment Engine
  ✓ Applications Resource Provider
  ✓ Controller
  ✓ ...
Performing post-upgrade verification...
  ✓ All components healthy

Upgrade complete! Radius has been successfully upgraded to v0.45.0.
Note: Your local Radius CLI is still v0.44.0. To upgrade your CLI, download the v0.45.0 version.
```

**Result:**

1. Pre-flight checks validate the upgrade is possible
2. All control plane components are upgraded in sequence
3. Post-upgrade verification confirms system health
4. User is notified about the CLI version mismatch

**Exceptions:**

1. If version check fails because the target is lower than current version
2. If components fail health checks after upgrade

#### Scenario 2: DevOps engineer upgrades with custom configuration

**User Story:** As a DevOps engineer, I need to upgrade Radius with custom configuration parameters to match organization's infrastructure requirements.

**User Experience:**

```bash
# Upgrade with custom configuration values
> rad upgrade kubernetes --version v0.44.0 --set global.monitoring.enabled=true --set global.resources.limits.memory=2Gi

Initiating Radius upgrade from v0.43.0 to v0.44.0 with custom configuration...
Custom configuration detected:
  - global.monitoring.enabled: true
  - global.resources.limits.memory: 2Gi
Pre-flight checks:
  ✓ Valid version target
  ✓ Compatible upgrade path
  ✓ Custom configuration validated
Upgrading control plane components with custom configuration:
  ✓ Universal Control Plane
  ✓ Deployment Engine
  ✓ Applications Resource Provider
  ✓ Controller
  ✓ ...
Applying custom configuration settings...
Performing post-upgrade verification...
  ✓ All components healthy
  ✓ Custom configuration applied successfully

Upgrade complete! Radius has been successfully upgraded to v0.44.0 with your custom configuration.
```

**Result:**

1. System acknowledges custom configuration parameters
1. Pre-flight checks validate both upgrade path and configuration
1. Control plane components are upgraded with custom settings
1. Verification confirms both health and configuration application

**Exceptions:**

1. If custom configuration parameters are invalid

#### Scenario 3: Handling upgrade failure and recovery

**User Story:** As a system administrator, I need confidence that if an upgrade fails, the system can recover without data loss or extended downtime.

**User Experience:**

```bash
# Attempt upgrade that encounters an issue
> rad upgrade kubernetes --version v0.44.0

Initiating Radius upgrade from v0.43.0 to v0.44.0...
Pre-flight checks:
  ✓ Valid version target
  ✓ Compatible upgrade path
Upgrading control plane components:
  ✓ Universal Control Plane
  ✓ Deployment Engine
  ✗ Applications Resource Provider (ERROR: Container image pull failed)

ERROR: Upgrade failed during Applications Resource Provider update.
Initiating automatic rollback to v0.43.0...
  ✓ Universal Control Plane reverted
  ✓ Deployment Engine reverted
  ✓ System verification complete
  ✓ ...

Rollback complete. System has been restored to v0.43.0.
Data snapshot 'snapshot-v0.43.0-20250305-093221' is available if data recovery is needed.
Review Kubernetes events and logs for more details on the failure.
```

**Result:**

1. System creates a data snapshot before starting the upgrade
2. System detects failure during the upgrade process
3. Helm-based rollback is initiated to revert Kubernetes resources
4. Control plane components are reverted to their previous version
5. User is informed about the available data snapshot for deeper recovery if needed

**Exceptions:**

1. If Helm rollback fails, manual intervention using the data snapshot may be required
2. If snapshot creation fails, the upgrade will not proceed

#### Scenario 4: Upgrading across multiple versions

**User Story:** As a DevOps engineer, I need to upgrade Radius from an older version to the latest version in a single operation.

**User Experience:**

```bash
# Check current version (significantly behind latest)
> rad version
RELEASE   VERSION   BICEP     COMMIT
0.40.0    v0.40.0   0.31.93   1dd17270ec9bc9e725f314fa62c249406034edda

# Upgrade directly to latest version
> rad upgrade kubernetes --version latest (Future Version)

Initiating Radius upgrade from v0.40.0 to v0.44.0 (latest stable)...
Pre-flight checks:
  ✓ Valid version target
  ✓ Multiple version jump detected (v0.40.0 → v0.44.0)
  ✓ Compatible upgrade path confirmed
  ✓ Database schema changes detected
Upgrading control plane components:
  ✓ Universal Control Plane
  ✓ Deployment Engine
  ✓ Applications Resource Provider
  ✓ Controller
Performing post-upgrade verification...
  ✓ All components healthy
  ✓ Database schema updated successfully

Upgrade complete! Radius has been successfully upgraded to v0.44.0.
Note: Your local Radius CLI is still v0.40.0. To upgrade your CLI, download the latest version.
```

**Result:**

1. System detects a multi-version upgrade scenario
1. Pre-flight checks validate that direct upgrade is supported
1. All components are upgraded to the target version
1. Verification confirms system health with the new version

**Exceptions:**

1. If direct upgrade path isn't supported between versions
1. If database migrations encounter issues (this may be the case when we introduce Postgres as the data store)
1. If intermediate upgrades are required first

## Design

### High-Level Design Diagram

```mermaid
graph TD
  CLI["Radius CLI (rad upgrade kubernetes)"] -->|Initiates Upgrade| KubernetesAPI["Kubernetes API"]
  KubernetesAPI -->|Manages Helm Charts| Helm["Helm"]
  Helm -->|Upgrades Components| ControlPlane["Radius Control Plane"]
  subgraph ControlPlane
      UniversalControlPlane["UCP"]
      DeploymentEngine["Deployment Engine"]
      ARP["Applications RP"]
      Controller["Controller"]
      DynamicRP["Dynamic RP"]
  end
  ControlPlane -->|Stores Metadata| Database["etcd"]
  ControlPlane -->|Interacts with| Dependencies["Dependencies (Dapr, Contour)"]
  CLI -->|Logs Progress| User["User"]
  CLI -->|Performs Pre-flight Checks| PreFlight["Pre-flight Checks"]
  PreFlight -->|Validates| KubernetesAPI
```

- **Important Note:** As of April 2025, Postgres is not fully implemented yet as the data store of Radius. We use etcd in production.

### Architecture Diagram

```mermaid
graph TD
    User([User]) --> CLI["Radius CLI"]
    CLI -->|"Initiates Upgrade"| K8sAPI["Kubernetes API"]

    K8sAPI --> PreflightChecks["Preflight Checks"]
    PreflightChecks --> ComponentUpgrade["Component Upgrade"]
    ComponentUpgrade --> PostUpgradeVerify["Post-Upgrade Verification"]

    subgraph "Radius Control Plane"
        UCP["Universal Control Plane"]
        DE["Deployment Engine"]
        ARP["Applications RP"]
        DynamicRP["Dynamic RP"]
        Controller["Controller"]
        Dashboard["Dashboard"]
    end

    ComponentUpgrade --> UCP
    ComponentUpgrade --> DE
    ComponentUpgrade --> ARP
    ComponentUpgrade --> DynamicRP
    ComponentUpgrade --> Controller
    ComponentUpgrade --> Dashboard

    UCP -->|"Reads/Writes"| Database["etcd / PostgreSQL"]
    UCP -.->|"Only holds metadata"| UserApps["User Applications"]
```

### Detailed Design

```mermaid
graph TD
    Start[User runs rad upgrade kubernetes] --> ParseArgs[Parse command arguments]
    ParseArgs --> ValidateVersion[Validate version compatibility]
    ValidateVersion --> AcquireLock[Acquire upgrade lock]
    AcquireLock --> RunPreflights[Run pre-flight checks]
    RunPreflights --> PlanUpgrade[Calculate upgrade plan]
    PlanUpgrade --> ExecuteHelmUpgrade[Execute Helm chart upgrade]
    ExecuteHelmUpgrade --> MonitorProgress[Monitor upgrade progress]
    MonitorProgress --> VerifyComponents[Verify component health]
    VerifyComponents --> Success{Successful?}
    Success -- Yes --> ReleaseLock[Release upgrade lock]
    Success -- No --> RollbackHelm[Rollback Helm changes]
    RollbackHelm --> ReleaseLock
    ReleaseLock --> End[Display results to user]
```

**Version Detection and Validation:**

The upgrade process begins with detecting the currently installed Radius version (the versions of the components in the cluster) and validating it against the requested target version. We can build an interface (or use/improve an existing one) to achieve this:

```go
// VersionValidator interface
type VersionValidator interface {
    ValidateTargetVersion(currentVersion, targetVersion string) error
    GetLatestVersion() (string, error)
    IsValidVersion(version string) bool
}
```

This interface will be implemented (or existing will be improved) to handle version comparisons, prevent downgrades, and resolve the "latest" version tag to a specific version number. The implementation will (probably) connect to the GitHub API to fetch available release versions when needed.

**Upgrade Lock Mechanism:**

- To prevent concurrent data‐modifying operations during `rad upgrade kubernetes`, we’ll rely exclusively on datastore locks (no Kubernetes leases).

```go
// UpgradeLock is implemented per datastore (Postgres, etcd) to serialize upgrades (with enhanced resilience)
type UpgradeLock interface {
    // AcquireLock obtains an exclusive lock with a TTL or fails
    AcquireLock(ctx context.Context, ttl time.Duration) error

    // ExtendLock refreshes the TTL on an existing lock (heartbeat)
    ExtendLock(ctx context.Context, ttl time.Duration) error

    // ReleaseLock explicitly releases a lock
    ReleaseLock(ctx context.Context) error

    // IsUpgradeInProgress checks if a valid lock exists
    IsUpgradeInProgress(ctx context.Context) (bool, error)

    // GetLockInfo returns metadata about the current lock
    GetLockInfo(ctx context.Context) (LockInfo, error)

    // ForceReleaseLock allows admin override with reason tracking
    ForceReleaseLock(ctx context.Context, reason string) error
}

type LockInfo struct {
    AcquiredAt      time.Time
    ExpiresAt       time.Time
    LockedBy        string
    LastHeartbeatAt time.Time
    IsStale         bool
}
```

**Timeouts:** callers must supply a context with a finite deadline (e.g. 2 min) to avoid blocking forever.
**Stale-lock detection:** each lock has a TTL/heartbeat; expired leases are auto-cleaned before AcquireLock.
Force cleanup: --force flag allows manual removal of stale/orphaned locks.

Usage in CLI commands:

```go
ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
defer cancel()

// attempt to acquire (with timeout + stale‐lock cleanup)
// AcquireLock must first scan for & remove any expired/orphaned locks before locking.
if err := upgradeLock.AcquireLock(ctx); err != nil {
    return fmt.Errorf("cannot start upgrade: %w", err)
}
defer func(){
    _ = upgradeLock.ReleaseLock(context.Background())
}()
```

We can utilize data-store-level lock mechanisms for implementing the distributed locking mechanism:

- PostgreSQL: <https://www.postgresql.org/docs/current/explicit-locking.html#ADVISORY-LOCKS>
- etcd: <https://etcd.io/docs/v3.5/tutorials/how-to-create-locks/>

Other CLI commands (`rad deploy app.bicep`, `rad delete app my-app` or other data-changing commands) that modify data will check for this lock before proceeding:

```go
inProgress, _ := upgradeLock.IsUpgradeInProgress(ctx)
if inProgress {
    return errors.New("An upgrade is currently in progress. Please try again after the upgrade completes.")
}
```

**Pre-flight Check System:**

Pre-flight checks run before any changes are made to ensure the upgrade can proceed safely.

```go
type PreflightCheck interface {
    Run(ctx context.Context) (bool, string, error)
    Name() string
    // Error, Warning, Info
    Severity() CheckSeverity
}
```

Checks will include:

1. Version compatibility verification
2. Existing installation detection
3. Database connectivity
4. Custom configuration validation

**[Future Version] User Data Backup and Restore System:**

Rather than taking complete snapshots of the underlying databases (etcd/PostgreSQL), we'll implement a more targeted approach that backs up only the user application metadata and configuration that Radius manages:

- **Included in backup**: User application, environment, recipe definitions, and all other resources that the user has deployed/added via Radius.
- **Not included in backup**: Anything other than user data in the data store.

```go
type UserDataBackup interface {
    // Creates a backup of all user application metadata and configurations
    BackupUserData(ctx context.Context) (BackupID string, err error)
    // Lists available user data backups with metadata
    ListBackups(ctx context.Context) ([]BackupInfo, error)
}

type UserDataRestore interface {
    // Restores user data from a previous backup
    RestoreUserData(ctx context.Context, backupID string) error
}
```

**Helm-based Upgrade Process:**

The core of the upgrade functionality will be implemented through Helm, leveraging its chart upgrade capabilities while adding Radius-specific safety measures:

```go
type HelmUpgrader interface {
    // Upgrades the Radius installation to a specified version
    UpgradeRadius(ctx context.Context, options UpgradeOptions) error

    // Returns the current status of an ongoing upgrade
    GetUpgradeStatus(ctx context.Context) (UpgradeStatus, error)

    // Validates that an upgrade to the target version is possible
    ValidateUpgrade(ctx context.Context, targetVersion string) error
}

type UpgradeOptions struct {
    Version string // Target version to upgrade to
    Values map[string]interface{} // Custom configuration values
    Timeout time.Duration // Maximum time allowed for upgrade

    EnableUserDataBackup  bool // Future Version: Whether automatic user data backup is enabled
    BackupID              string // Future Version: ID of user data backup to use for recovery
}
```

**Component Health Verification:**

After upgrades, the system verifies all components are healthy:

```go
type HealthChecker interface {
    CheckComponentHealth(ctx context.Context, component string) (bool, error)
    CheckAllComponents(ctx context.Context) (map[string]ComponentHealth, error)
    WaitForHealthyState(ctx context.Context, timeout time.Duration) error
}

type ComponentHealth struct {
    Status      HealthStatus
    Message     string
    ReadinessDetails map[string]string
    LastChecked time.Time
}

type HealthStatus string

const (
    StatusHealthy     HealthStatus = "Healthy"
    StatusDegraded    HealthStatus = "Degraded"
    StatusUnavailable HealthStatus = "Unavailable"
    StatusUnknown     HealthStatus = "Unknown"
)
```

#### Advantages of this approach

1. **User Experience**: Provides a single command for upgrading all Radius components, significantly simplifying the process compared to manual uninstall/reinstall
1. **Safety**: Built-in user data backup and restore capabilities ensure user data is protected during upgrades
1. **Flexibility**: Support for custom configuration parameters allows adaptation to different environments
1. **Transparency**: Clear, step-by-step output keeps users informed of the upgrade process
1. **Consistency**: Ensures all Radius components are upgraded together to compatible versions
1. **Safety**: Comprehensive preflight checks prevent upgrades in unsuitable conditions, while built-in user data backup and restore capabilities ensure user data is protected during upgrades

#### Disadvantages of this approach

1. **Additional Complexity**: Implementing backup/restore functionality adds complexity to the codebase
1. **Limited Control**: Users have less granular control compared to manually upgrading components
1. **Resource Requirements**: The upgrade process temporarily requires additional resources during the transition period
1. **Upgrade Path Constraints**: Some version combinations may require intermediate upgrades, limiting direct jump capability
1. **CLI Version Mismatch**: Potential confusion for users when their CLI version doesn't match the server version

#### Proposed Option

I recommend implementing the helm-based upgrade approach with automatic user data backup/restore functionality (we can even discuss if we would like this functionality in version 1 or not) because:

1. It provides the best balance between user experience simplicity and technical safety
2. Leverages existing Helm functionality while adding Radius-specific safety features
3. The automated user data backup/restore mechanism offers critical protection against data loss
4. Clear progress indicators and health checks give users confidence in the process
5. This approach aligns with how users currently install/reinstall Radius, providing consistency

### API design (if applicable)

No specific REST API addition is necessary.

### CLI Design (if applicable)

Main CLI changes:

- Addition of `rad upgrade kubernetes` command (details mentioned above)
- Lock logic will be added to the existing commands that change user data like:
  - `rad deploy app.bicep`
  - `rad app delete my-app`
  - `rad env delete default`

### Implementation Details

The implementation will primarily focus on the following components:

1. **Upgrade Command**: The `rad upgrade kubernetes` command implementation in the CLI codebase
2. **Version Validation**: Logic to verify compatibility between versions
3. **Lock Mechanism**: Data-store-level distributed locking system
4. **Data Snapshot**: Lightweight snapshot mechanism for etcd/PostgreSQL before upgrades (in one of the upcoming versions of `rad upgrade kubernetes`)
5. **Preflight Checks**: Validation system to ensure prerequisites are met before upgrade
6. **Helm Integration**: Enhanced wrapper around Helm's upgrade capabilities
7. **Health Verification**: Component readiness and health check mechanisms
8. **[Future Version] Backup/Restore**: User data protection system using ConfigMaps/PVs

All components will follow Radius coding standards and include comprehensive unit tests.

### Error Handling

The upgrade process will implement the following error handling strategies:

1. **Pre-flight Validation**: Catch incompatibility issues before starting the upgrade.
2. **Graceful Timeouts**: All operations will respect user-defined or default timeouts.
3. **Two-tier Rollback Strategy**:
   1. **Helm-based Rollback**: For version 1, failed upgrades will leverage Helm's built-in rollback capability to revert Kubernetes resources to their previous state.
   2. **Data Snapshot Rollback**: In one of the upcoming versions, before starting the upgrade, the system will take a snapshot of the data store and label it with the current version. If critical issues are discovered after the upgrade, administrators can restore from this snapshot.
      - **etcd**: Will use etcd's built-in snapshot functionality (<https://etcd.io/docs/v3.5/op-guide/recovery/>)
      - **PostgreSQL**: Will use `pg_dump` or equivalent backup mechanisms
      - **Important**: After restoring from a snapshot, the application graph will reflect the state at the time of the snapshot. Users must run a deployment after rollback to ensure their applications match the current desired state.
4. **Detailed Error Reporting**: Clear error messages with troubleshooting guidance.
5. **Idempotent Operations**: Commands can be safely retried after addressing issues.

**Important Note**: Data snapshot rollback is distinctly different from migration rollback:

- **Migration rollback**: Reverses schema changes by running down migrations, preserving recent data while changing structure
- **Data snapshot rollback**: Restores the entire datastore to a previous point in time, losing any changes made after the snapshot
- **Post-rollback requirement**: Since snapshots capture the state at a specific time, any deployments or changes made after the snapshot will be lost. Users must redeploy their applications after a snapshot rollback to ensure consistency.

## Test Plan

### Unit Tests

- Test each interface implementation independently
- Test each preflight check with various input scenarios (pass/fail/warning)
- Test preflight check registry with multiple checks of different severities

### End-to-End Tests

- Perform upgrades across consecutive versions (e.g., v0.43 → v0.44)
- Test version skipping scenarios (e.g., v0.40 → v0.44)
- Verify custom configuration is properly applied
- Simulate failures and verify automatic rollback

## Security

No changes to the existing security model needed. However, when the backup/restore functionality is implemented in future versions, several security considerations will apply:

- **Backup storage location**: User data backups will be stored as Kubernetes resources (ConfigMaps or PersistentVolumes) within the same namespace as the Radius installation, inheriting the cluster's security boundaries.
- **Access control**: Only users with appropriate Kubernetes RBAC permissions to the Radius namespace will be able to access or manage these backups, following the principle of least privilege.
- **Backup lifecycle**: Backups will have configurable retention policies with automatic pruning of older backups after successful upgrades, preventing accumulation of sensitive historical data.
- **Data sensitivity**: Since backups contain only metadata about user applications (not the applications themselves), the security risk is limited to configuration exposure rather than direct workload compromise.

## Monitoring and Logging

- Upgrade operations will emit detailed logs indicating progress, success, or failure of each step.
- Metrics can be collected to track upgrade success rates, duration, and rollback occurrences.
- Users will have clear visibility into upgrade status through CLI output and Kubernetes events.

## Development Plan

The following outlines the key implementation steps required to deliver the Radius Control Plane upgrade feature. Each step includes necessary unit and functional tests to ensure reliability and correctness, along with dependency information.

### Version 1: Simple `rad upgrade kubernetes` command with incremental upgrade

1. **Radius Helm Client Updates**

   - Implement the upgrade functionality in the Radius Helm client: [helmclient.go](https://github.com/radius-project/radius/blob/main/pkg/cli/helm/helmclient.go).
   - Add unit tests to validate Helm upgrade logic.
   - This task can be worked on in parallel with items 2-3, 4-5. It is a blocker for item 6.

2. **Radius Contour Client Updates**

   - Implement the upgrade functionality in the Radius Contour client: [contourclient.go](https://github.com/radius-project/radius/blob/main/pkg/cli/helm/contourclient.go).
   - Add unit tests to verify correct behavior.
   - This task can be worked on in parallel with items 1, 3, 4-5. It is a blocker for item 6.

3. **Cluster Upgrade Interface**

   - Extend the existing cluster management interface ([cluster.go](https://github.com/radius-project/radius/blob/main/pkg/cli/helm/cluster.go#L249)) to include a new method for upgrading Radius.
   - Implement this method in all relevant interface implementations.
   - Integrate with version validation and custom configuration handling.
   - Add comprehensive unit tests for this functionality.
   - This task can be worked on in parallel with items 1-2, 4-5. It is a blocker for item 6.

4. **Upgrade Lock Mechanism**

   - Implement the upgrade lock interface to prevent concurrent modifications.
   - Update existing CLI commands to check for locks before data modification.
   - Can be implemented in parallel with items 1-3, 5. Required for item 6.

5. **Preflight Checks Implementation**

   - Implement the `PreflightCheck` interface and create concrete check implementations:
     - **VersionCompatibilityCheck**: Validates target version is newer than current version
     - **ClusterResourceCheck**: Verifies the cluster has sufficient resources (CPU, memory)
     - **ControlPlaneHealthCheck**: Confirms current installation is in a healthy state
     - **CustomConfigValidationCheck**: Validates any custom configuration parameters
   - Create a preflight checks registry to manage and execute checks in sequence
   - Implement severity levels (Error, Warning, Info) and appropriate user feedback
   - Add unit tests for each check implementation
   - This task can be implemented in parallel with items 1-4 and is required for item 6.

6. **CLI Command Implementation**

   - Implement the `rad upgrade kubernetes` command, integrating all previously defined components and interfaces.
   - Ensure the command performs pre-flight checks, component upgrades, Helm-based rollback on failure, and post-upgrade verification.
   - Include detailed CLI output and logging for user visibility.
   - Add necessary unit and functional tests to validate command behavior.
   - This task depends on all previous tasks (1-5) and should be implemented last.

### Future Versions

#### Data Snapshot Support

1. **Snapshot Implementation**

   - For etcd: Implement snapshot using etcd's recovery API (<https://etcd.io/docs/v3.5/op-guide/recovery/>)
   - For PostgreSQL: Implement using `pg_dump` or similar
   - Label snapshots with the current version before upgrade

2. **Restore Documentation**
   - Document manual restore procedures for emergency recovery
   - Include warning that snapshots reflect the application graph at the time of backup
   - Clearly state that users must redeploy applications after snapshot restore to ensure consistency

#### Data Store Migrations and Rollbacks

1. Pick & embed a migration tool

   - Add `migrations/` dir and versioned SQL (or Go) files
   - Vendor or import the tool (e.g. golang-migrate) so we have a single binary

2. Define migration tracking schema

   - For Postgres: create a `schema_migrations` table (tool-standard)
   - For etcd: track applied migrations via a reserved key prefix

3. Wiring in the CLI/server

   - On install/upgrade: run `migrate up` before Helm chart upgrade
   - On rollback: run `migrate down` (or tool-provided rollback) if upgrade fails
   - (Optional) Expose `rad migrate status|up|down` commands for operators

4. Schema evolution helpers

   - Provide utilities for common ops (add column, rename, rekey in etcd)
   - Write examples/migrations: e.g. move key-value → Postgres row

5. Testing migrations

   - Unit tests for each migration file (idempotent, up/down)
   - Integration tests: start with old schema + data → apply migrations → verify shape

6. Documentation & patterns

   - Doc: “How to write a new migration”
   - Versioning rules (major/minor jumps, compatibility guarantees)
   - Rollback advice: when to write reversible vs. irreversible migrations

#### Integrate User Data Backup and Restore

1. **User Data Backup and Restore Interfaces**

   - Define two new interfaces in the `components/database` package:
     - `UserDataBackup`: Responsible for creating backups of user data before the upgrade.
     - `UserDataRestore`: Responsible for restoring data from the backup in case of rollback.
   - Design versioned backup formats to handle schema migrations between versions.
   - This task can be worked on in parallel with items 1-3. It's a blocker for item 5.

2. **User Data Backup and Restore Implementation**

   - Implement the backup and restore interfaces in the following data store implementations:
     - **In-memory datastore**: [inmemory/client.go](https://github.com/radius-project/radius/blob/main/pkg/components/database/inmemory/client.go)
     - **Postgres datastore**: [postgresclient.go](https://github.com/radius-project/radius/blob/main/pkg/components/database/postgres/postgresclient.go)
   - Add comprehensive unit tests for each implementation.
   - Implement backup storage mechanism in Kubernetes (ConfigMaps or PVs depending on size).
   - This task depends on item 4 (interfaces) and blocks item 6 (CLI implementation).

#### Rollback to the most recent successful version of Radius

1. **Version History Tracking**

   - Extend the backup system to record the last successful control-plane version (e.g. in a reserved etcd key or Postgres table).
   - Ensure every successful `rad upgrade kubernetes` run writes an entry with timestamp and version.

2. **`rad rollback` CLI Command**

   - Introduce `rad rollback kubernetes` that reads the recorded “last known good” version and invokes the same upgrade path in reverse.
   - Integrate rollback into the existing lock and backup/restore interfaces.

3. **Stateful Rollback Validation**

   - Implement post-rollback health checks (component health, data-integrity assertions).
   - Fail early if rollback target is stale or schema mismatches prevent safe restoration.

4. **End-to-End Test Matrix**
   - Add scenarios: v0.43 → v0.44 upgrade → failure → `rad rollback` → verify control plane matches pre-upgrade state.
   - Test edge cases where no previous version is recorded.

#### Skip versions during `rad upgrade kubernetes`

1. **Skip-Aware Pre-flight Checks**

   - Enhance `VersionValidator` to detect multi-version jumps and verify compatibility (e.g. migrations available).
   - Warn or block skips if there are known incompatible intermediate releases.

2. **Migration Plan Bundles**

   - Generate a composite plan when skipping (e.g. v0.42 → v0.45):  
     • List required data migrations in sequence  
     • Group Helm chart upgrades and backup points for each intermediate step

3. **User Confirmation & Dry-Run**

   - Prompt the user with a clear “You're jumping from A→D. We'll run migrations for B and C in turn. Proceed?”
   - Offer a `--dry-run` mode that prints the full step list without making changes.

4. **Automated Integration Tests**

   - Cover a variety of version skip paths in CI (adjacent vs. multi-minor).
   - Fail if any migration or Helm chart upgrade in the skip path is missing.

#### Support for Air-Gapped Environments

This can be discussed later.

#### Upgrading Radius on other platforms like `rad upgrade aci`

This can be discussed later.

### Out of Scope for Implementation

- **Dry-run functionality**: After team discussion, the dry-run feature was explicitly excluded from this implementation.
- **Dependency upgrades**: Upgrading major versions of dependencies (Contour, Dapr, Postgres) is explicitly out of scope and should be handled separately.
- **Automatic CLI upgrades**: Updating the local Radius CLI automatically is not included; users must manually update their CLI version separately.

### Implementation Risks and Mitigations

- **Rollback Reliability**: Helm-based rollback mechanisms should be thoroughly tested to ensure they can return the control plane to a working state if upgrades fail.
- **Lock Persistence**: Ensure upgrade locks have proper timeout mechanisms to avoid permanently locked systems if a process terminates unexpectedly.

### Testing Strategy

- **Unit Tests**: Cover all new code paths, especially version validation, upgrade logic, lock mechanisms, and error handling.
- **Functional Tests**: Validate end-to-end upgrade scenarios, including successful upgrades, upgrades with custom configurations, failure scenarios, and Helm-based rollback procedures.
- **Compatibility Tests**: Verify compatibility between different Radius CLI versions and control plane components.

## Open Questions

- **Rollback reliability**: What specific scenarios could cause rollback to fail, and how should we mitigate these risks?
  - Do we need rollbacks in version 1?
  - If the answer to the item above is yes, then do we need to add the implementation of the interfaces for Postgres in version 1?
- **Version skipping limits**: Should we enforce incremental upgrades for certain major version jumps, or always allow direct version skipping?
- **Upgrade notifications**: How should we notify users clearly about the CLI version mismatch after upgrading the control plane?
- **Resource constraints**: How do we handle scenarios where the cluster lacks sufficient resources to perform a rolling upgrade?

## Design Review Notes

- Create a task for adding `How to do upgrade` to the design document template: <https://github.com/radius-project/design-notes/pull/87#discussion_r2032112381>.
- Create a task to publish the resource requirements of `rad upgrade kubernetes` in the Radius docs: <https://github.com/radius-project/design-notes/pull/87#discussion_r2032080262>.
- Create a task to point the users to the relevant docs page for upgrading CLI and Control Plane: <https://github.com/radius-project/design-notes/pull/87#discussion_r2032085654>.
- Create a task to write a documentation on manually rolling back to an older version: <https://github.com/radius-project/design-notes/pull/87#discussion_r2032079932>.
-

## Topics to discuss

- Incremental Upgrades
- Lock in the Data Store layer
- Compatibility between versions logic
- Backup data storage location
- Database migrations
