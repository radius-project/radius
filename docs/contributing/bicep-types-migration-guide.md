# Migration Guide: Removing bicep-types Submodule

This guide is for developers who have an existing clone of the Radius repository that includes the `bicep-types` git submodule. After this migration, the submodule is no longer used—bicep-types dependencies are now managed via pnpm.

## What Changed

- **No more submodule**: The `bicep-types` git submodule has been removed
- **pnpm for JavaScript dependencies**: All JavaScript/TypeScript projects now use pnpm instead of npm
- **Automatic dependency fetch**: The `bicep-types` package is now fetched as a git dependency and built via a `postinstall` script
- **Simpler cloning**: New clones no longer require `--recurse-submodules` flag

## Prerequisites

### Remove global TypeSpec compiler

TypeSpec (`@typespec/compiler`) should **not** be installed globally. A global installation can cause version conflicts and unexpected behavior. Typespec should be installed locally in the `typespec` folder via pnpm so that the correct version is used.

To check if you have TypeSpec installed globally:

```bash
npm list -g @typespec/compiler
```

If it is installed globally, remove it:

```bash
npm uninstall -g @typespec/compiler
```

Then ensure the local installation is available by running:

```bash
pnpm -C typespec install
```

You can run `make generate-tsp-installed` to verify that the local TypeSpec installation is working correctly.

## Installing Dependencies

After the migration, install dependencies using pnpm:

```bash
# Install pnpm if not already installed
npm install -g pnpm

# Install typespec dependencies
pnpm -C typespec install
```

## Verifying the Migration

Run the build to verify everything works:

```bash
make generate
```

If you need to regenerate Bicep types: (note: this target is included in `make generate`)

```bash
make generate-bicep-types
```

The above commands should complete without errors and without new pending changes in git.

## Switching Between Branches

When switching between branches that have the submodule and branches that don't, you may encounter errors like:

```
fatal: not a git repository: ../.git/modules/bicep-types
fatal: could not reset submodule index
```

**These errors are recoverable—you do not need to re-clone the repository.**

The solution is to use `git checkout --no-recurse-submodules` and clean up stale submodule artifacts before switching.

### Switching Branches (Recommended Procedure)

Use this procedure when switching between branches with different submodule configurations:

```bash
# Navigate to your radius repository
cd /path/to/radius

# Clean up any stale submodule artifacts
rm -rf bicep-types
rm -rf .git/modules/bicep-types

# Switch branches with --no-recurse-submodules flag
git checkout --no-recurse-submodules <target-branch>
```

### Recover From a Failed Checkout

If you already attempted a checkout and it failed, git may be in an inconsistent state. You might see many unexpected file changes when running `git status`. To recover:

```bash
# Navigate to your radius repository
cd /path/to/radius

# Reset your working tree to the current HEAD
git reset --hard HEAD

# Remove the submodule directory and git's cached submodule data
rm -rf bicep-types
rm -rf .git/modules/bicep-types

# Remove any stale .gitmodules file that may have appeared
rm -f .gitmodules

# Remove submodule config entry (if it exists)
git config --local --remove-section submodule.bicep-types 2>/dev/null || true

# Now switch to the target branch with --no-recurse-submodules
git checkout --no-recurse-submodules <target-branch>
git pull
```

### Verify Clean State

After switching branches, verify the repository is in a clean state:

```bash
git status
# Should show "nothing to commit, working tree clean"
```

## Troubleshooting

### "bicep-types not found" errors

If you see errors about `bicep-types` not being found:

1. Ensure you've run `pnpm install` in the appropriate directories
2. The `postinstall` script should automatically build bicep-types and create a symlink

### "pnpm not found" errors

Install pnpm globally:

```bash
npm install -g pnpm
```

Or if using corepack (Node.js 16.13+):

```bash
corepack enable
corepack prepare pnpm@10 --activate
```

### Stale submodule references

If git still shows the submodule or you encounter submodule-related errors, see [Switching Between Branches](#switching-between-branches) for the complete recovery procedure.

## Questions?

If you encounter issues with this migration, please open an issue at <https://github.com/radius-project/radius/issues>.
