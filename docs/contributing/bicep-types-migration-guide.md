# Migration Guide: Removing bicep-types Submodule

This guide is for developers who have an existing clone of the Radius repository that includes the `bicep-types` git submodule. After this migration, the submodule is no longer used—bicep-types dependencies are now managed via pnpm.

## One-Time Migration Steps

If you cloned the repository before this migration, run the following commands to clean up your local copy:

```bash
# Navigate to your radius repository
cd /path/to/radius

# Remove the submodule from git's index (if it still exists)
git rm --cached bicep-types 2>/dev/null || true

# Remove the submodule directory
rm -rf bicep-types

# Clean up git modules directory
rm -rf .git/modules/bicep-types

# Update to get latest changes
git fetch origin
git checkout main
git pull

# Verify the submodule is removed
git submodule status
# Should show no submodules
```

## Installing Dependencies

After the migration, install dependencies using pnpm:

```bash
# Install pnpm if not already installed
npm install -g pnpm

# Install typespec dependencies
cd typespec
pnpm install
cd ..

# Install bicep types generator dependencies (if you need to run code generation)
cd hack/bicep-types-radius/src/generator
pnpm install
cd ../autorest.bicep
pnpm install
cd ../../../..
```

## Verifying the Migration

Run the build to verify everything works:

```bash
make build
```

If you need to regenerate Bicep types:

```bash
make generate-bicep-types
```

## What Changed

- **No more submodule**: The `bicep-types` git submodule has been removed
- **pnpm for JavaScript dependencies**: All JavaScript/TypeScript projects now use pnpm instead of npm
- **Automatic dependency fetch**: The `bicep-types` package is now fetched as a git dependency and built via a `postinstall` script
- **Simpler cloning**: New clones no longer require `--recurse-submodules` flag

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

If git still shows the submodule:

```bash
git rm --cached bicep-types
rm -rf .git/modules/bicep-types
rm -rf bicep-types
```

## Questions?

If you encounter issues with this migration, please open an issue at https://github.com/radius-project/radius/issues.
