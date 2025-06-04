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

### Usage Example

```go
import (
    "context"
    "github.com/radius-project/radius/pkg/upgrade/preflight"
    "github.com/radius-project/radius/pkg/cli/output"
)

func validateUpgrade(ctx context.Context, output output.Interface, currentVersion, targetVersion string) error {
    // Create preflight check registry
    registry := preflight.NewRegistry(output)

    // Add checks to registry
    registry.AddCheck(preflight.NewVersionCompatibilityCheck(currentVersion, targetVersion))

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
