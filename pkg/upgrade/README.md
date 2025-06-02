# Radius Upgrade Package

This package contains utilities and components for upgrading Radius installations across different platforms and contexts.

## Structure

- `preflight/` - Pre-upgrade validation checks that can be used by any component

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
