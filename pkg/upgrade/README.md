# Radius Upgrade Package

This package contains utilities and components for upgrading Radius installations across different platforms and contexts.

## Structure

- `preflight/` - Pre-upgrade validation checks that can be used by any component
- `preupgrade/` - Runs the configured preflight checks before an upgrade
- `hashmigration/` - Background migration of values that older versions derived with SHA-1, run by each control-plane component after an upgrade

## Preflight Checks

The preflight checks are designed to be reusable across different upgrade contexts:

- **CLI Commands** - `rad upgrade kubernetes` command
- **Controllers** - Kubernetes controllers managing Radius upgrades
- **APIs** - REST APIs that trigger upgrade operations
- **Automated Systems** - CI/CD or GitOps systems managing Radius

### Available Checks

Currently implemented:

1. **VersionCompatibilityCheck** - Validates version upgrade paths and prevents downgrades
2. **RadiusInstallationCheck** - Verifies Radius is currently installed and healthy
3. **KubernetesConnectivityCheck** - Tests cluster connectivity and permissions
4. **KubernetesResourceCheck** - Checks cluster resource availability for upgrades
5. **HelmConnectivityCheck** - Verifies Helm can access the cluster and find Radius release
6. **CustomConfigValidationCheck** - Validates --set and --set-file parameters

### Usage Example

```go
import (
    "context"
    "github.com/radius-project/radius/pkg/upgrade/preflight"
    "github.com/radius-project/radius/pkg/cli/output"
    "github.com/radius-project/radius/pkg/cli/helm"
)

func validateUpgrade(ctx context.Context, output output.Interface, helmInterface helm.Interface,
    currentVersion, targetVersion, kubeContext string, setParams, setFileParams []string) error {
    // Create preflight check registry
    registry := preflight.NewRegistry(output)

    // Add checks to registry in order of importance
    registry.AddCheck(preflight.NewKubernetesConnectivityCheck(kubeContext))
    registry.AddCheck(preflight.NewHelmConnectivityCheck(helmInterface, kubeContext))
    registry.AddCheck(preflight.NewRadiusInstallationCheck(helmInterface, kubeContext))
    registry.AddCheck(preflight.NewVersionCompatibilityCheck(currentVersion, targetVersion))
    registry.AddCheck(preflight.NewCustomConfigValidationCheck(setParams, setFileParams))
    registry.AddCheck(preflight.NewKubernetesResourceCheck(kubeContext))

    // Run all checks - registry handles execution and logging
    results, err := registry.RunChecks(ctx)
    if err != nil {
        return fmt.Errorf("preflight checks failed: %w", err)
    }

    // All checks passed
    return nil
}
```

### Extensibility

The preflight check system is designed to be extensible. New checks can be added by implementing the `PreflightCheck` interface:

```go
type PreflightCheck interface {
    Run(ctx context.Context) (bool, string, error)
    Name() string
    Severity() CheckSeverity
}
```

## SHA-256 Migration

`hashmigration` moves values that older versions of Radius derived with SHA-1, such as datastore object names, tracked-resource IDs, Terraform state names, and container secret hashes, to their SHA-256 form. UCP, Applications RP, and Dynamic RP each run a `hashmigration.Service` with the steps for the data they own. The service retries until every step is done and records the status of each step in the `radius-sha256-migration` ConfigMap in `radius-system`. The release that removes SHA-1 must check that status first, and accept only statuses that the installed release recorded after it was last deployed, because a status recorded before a rollback can be stale. See the [design note](../../eng/design-notes/security/2026-06-sha1-to-sha256-migration.md#automatic-migration).
