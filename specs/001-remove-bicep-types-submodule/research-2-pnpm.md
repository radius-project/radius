# Research: pnpm Migration and Submodule Removal

**Plan**: [plan-2-pnpm-submodule.md](./plan-2-pnpm-submodule.md)
**Date**: 2026-01-22 (Updated: 2026-01-31)
**Status**: Complete (Validated via prototype)

## Research Questions

From Plan 2, the following items required research:

1. pnpm Git Reference Syntax - Exact syntax for subdirectory git references
2. pnpm + Dependabot - Integration and configuration
3. npm to pnpm Migration - Best practices and commands
4. pnpm in GitHub Actions - Installation and caching strategies
5. Dev Container pnpm - Installation method for dev containers
6. bicep-types npm Package - Verify package structure and build requirements
7. **NEW** TypeScript Build Strategy - How to build bicep-types in node_modules

---

## Findings

### 1. pnpm Git References for TypeScript Packages

| Aspect | Details |
| ------ | ------- |
| **Decision** | Reference full repo via `git+https://` URL in package.json; pnpm fetches it automatically during install; build via postinstall script; create symlink |
| **Evidence** | Prototype validation on branch `brooke-hamilton:radius:pnpm-direct-ref` |
| **Rationale** | pnpm's `#path:/` subdirectory syntax does NOT work for TypeScript packages that require compilation; the package must be built in place after pnpm fetches it |
| **Alternatives Considered** | `github:...#path:/src/bicep-types` - does not work because TypeScript needs compilation; npm tarball from GitHub releases - not available |

**❌ Syntax That Does NOT Work:**

```json
// These do NOT work for TypeScript packages requiring build
"bicep-types": "github:Azure/bicep-types#path:/src/bicep-types"
"bicep-types": "github:Azure/bicep-types#c1a289be58be&path:/src/bicep-types"
```

**Why pnpm subdirectory references don't work:**

- The `bicep-types` package is TypeScript source that must be compiled
- pnpm installs the source files but does NOT run `npm install` or `npm run build`
- The package's `lib/` directory (compiled output) does not exist after install

**✅ Recommended Approach (Validated in Prototype):**

Reference the full repo in package.json. pnpm fetches it to `node_modules/bicep-types-repo/` during install. A postinstall script then builds the TypeScript and creates a symlink:

```json
{
  "pnpm": {
    "onlyBuiltDependencies": ["autorest"]
  },
  "scripts": {
    "postinstall": "cd node_modules/bicep-types-repo/src/bicep-types && npm install && npm run build && cd ../../../.. && rm -rf node_modules/bicep-types && ln -sf bicep-types-repo/src/bicep-types node_modules/bicep-types"
  },
  "devDependencies": {
    "bicep-types-repo": "git+https://github.com/Azure/bicep-types.git#556bf5edad58e47ca57c6ddb1af155c3bcfdc5c7"
  }
}
```

**Key elements:**

| Element | Purpose |
| ------- | ------- |
| `bicep-types-repo` package name | pnpm installs the full repo to `node_modules/bicep-types-repo/`; name differs from symlink target |
| `git+https://` URL format | Required by pnpm (not `github:` shorthand which defaults to SSH) |
| Commit SHA after `#` | Pins to specific version |
| `postinstall` script | Builds TypeScript and creates symlink after pnpm fetches the repo |
| `pnpm.onlyBuiltDependencies` | Allows autorest lifecycle scripts (matches npm behavior) |

**Postinstall script breakdown:**

```bash
# 1. Navigate to the bicep-types package within the fetched repo
cd node_modules/bicep-types-repo/src/bicep-types

# 2. Install its dependencies and compile TypeScript
npm install && npm run build

# 3. Return to package root
cd ../../../..

# 4. Remove any existing bicep-types directory/symlink
rm -rf node_modules/bicep-types

# 5. Create symlink so "import from 'bicep-types'" resolves correctly
ln -sf bicep-types-repo/src/bicep-types node_modules/bicep-types
```

**Note:** An `.npmrc` file with `side-effects-cache = false` is required to ensure postinstall scripts run correctly.

---

### 2. pnpm + Dependabot Integration

| Aspect | Details |
| ------ | ------- |
| **Decision** | Use `package-ecosystem: "npm"` for pnpm projects; git dependencies require manual updates |
| **Evidence** | [GitHub Dependabot docs](https://docs.github.com/code-security/dependabot) - pnpm v7-v10 lockfiles supported under npm ecosystem; prototype validation |
| **Rationale** | Dependabot treats pnpm as npm-compatible; git-based deps (`git+https://...#commit`) have NO auto-update support |
| **Alternatives Considered** | Renovate bot - more pnpm-native but adds complexity; manual updates only - reduces automation |

**Configuration (from prototype):**

```yaml
# .github/dependabot.yml
version: 2
updates:
  # For autorest.bicep directory (NEW)
  - package-ecosystem: npm
    directory: /hack/bicep-types-radius/src/autorest.bicep
    schedule:
      interval: weekly
    groups:
      autorest-bicep:
        patterns:
          - "*"

  # For generator directory (NEW)
  - package-ecosystem: npm
    directory: /hack/bicep-types-radius/src/generator
    schedule:
      interval: weekly
    groups:
      bicep-generator:
        patterns:
          - "*"

  # For typespec directory (KEEP existing)
  - package-ecosystem: npm
    directory: /typespec
    schedule:
      interval: weekly
    groups:
      typespec:
        patterns:
          - "*"
```

**Limitations:**

| Feature | Support |
| ------- | ------- |
| pnpm lockfile updates | ✅ Supported (v7-v10) |
| Registry package updates | ✅ Full support |
| `git+https://...#commit` dependency updates | ❌ NOT supported |
| Security alerts | ✅ Supported |

**Workaround for Git Dependencies:**

The `bicep-types-repo` commit SHA must be updated manually. Consider a scheduled GitHub Action:

```yaml
name: Check bicep-types updates
on:
  schedule:
    - cron: '0 0 * * 0'  # Weekly
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: |
          CURRENT=$(git ls-remote https://github.com/Azure/bicep-types HEAD | cut -f1)
          echo "Latest bicep-types commit: $CURRENT"
          # Compare with pinned version and create issue if different
```

---

### 3. npm to pnpm Migration

| Aspect | Details |
| ------ | ------- |
| **Decision** | Fresh `pnpm install` after updating package.json (no import needed due to dependency changes) |
| **Evidence** | Prototype validation - dependencies change significantly with git reference |
| **Rationale** | The bicep-types reference changes from `file:` to `git+https://`; clean install is appropriate |
| **Alternatives Considered** | `pnpm import` - not suitable when dependencies fundamentally change |

**Migration Steps (from prototype):**

```bash
# Per-directory migration for bicep-types-radius packages
cd hack/bicep-types-radius/src/autorest.bicep/

# 1. Update package.json with:
#    - pnpm.onlyBuiltDependencies: ["autorest"]
#    - postinstall script
#    - bicep-types-repo git reference (replacing bicep-types file: reference)

# 2. Create .npmrc with side-effects-cache = false

# 3. Delete old lockfile and node_modules
rm -rf package-lock.json node_modules

# 4. Install with pnpm (generates pnpm-lock.yaml)
pnpm install

# 5. Verify build works
pnpm run build

# Repeat for:
# - hack/bicep-types-radius/src/generator/
```

**For typespec/ (no bicep-types dependency):**

```bash
cd typespec/

# 1. Import existing lockfile (converts package-lock.json → pnpm-lock.yaml)
pnpm import

# 2. Install dependencies with pnpm (validates the import)
pnpm install

# 3. Verify
pnpm test

# 4. Remove old lockfile
rm package-lock.json
```

**Supported Import Sources:**

- `package-lock.json` (npm v5+) ✅
- `npm-shrinkwrap.json` ✅
- `yarn.lock` ✅

**Key Differences from npm:**

| Aspect | npm | pnpm |
| ------ | --- | ---- |
| node_modules structure | Flat | Symlinked from store |
| Disk usage | Duplicated | Content-addressable (shared) |
| Install speed | Slower | Faster |
| Phantom dependencies | Allowed | Blocked by default |
| Lock file | package-lock.json | pnpm-lock.yaml |
| Git repo packages | `package.json` at root required | Full repo fetched to node_modules/, postinstall handles build |

---

### 4. pnpm in GitHub Actions

| Aspect | Details |
| ------ | ------- |
| **Decision** | Use `pnpm/action-setup@v4` with `actions/setup-node@v4` caching |
| **Evidence** | [github.com/pnpm/action-setup](https://github.com/pnpm/action-setup) |
| **Rationale** | Official pnpm action with built-in store caching |
| **Alternatives Considered** | Manual npm install of pnpm - slower and no caching benefits |

**Recommended Configuration:**

```yaml
- name: Install pnpm
  uses: pnpm/action-setup@v4
  with:
    version: 10  # or specific: 10.8.1

- name: Setup Node.js
  uses: actions/setup-node@v4
  with:
    node-version-file: '.node-version'
    cache: 'pnpm'  # Built-in pnpm cache support

- name: Install dependencies
  run: pnpm install --frozen-lockfile
```

**Action Features:**

| Option | Description | Recommended |
| ------ | ----------- | ----------- |
| `version` | pnpm version (10, 10.x, 10.8.1) | `10` |
| `run_install` | Auto-run install | `false` (explicit is better) |
| Built-in cache | Automatic store caching | Uses setup-node cache |

**Full Workflow Example:**

```yaml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Install pnpm
        uses: pnpm/action-setup@v4
        with:
          version: 10
      
      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version-file: '.node-version'
          cache: 'pnpm'
          cache-dependency-path: |
            typespec/pnpm-lock.yaml
            hack/bicep-types-radius/src/*/pnpm-lock.yaml
      
      - name: Install TypeSpec dependencies
        run: pnpm --prefix typespec install --frozen-lockfile
      
      - name: Install generator dependencies
        run: pnpm --prefix hack/bicep-types-radius/src/generator install --frozen-lockfile
```

---

### 5. Dev Container pnpm Installation

| Aspect | Details |
| ------ | ------- |
| **Decision** | Use official Node.js feature with Corepack activation |
| **Evidence** | [containers.dev/features](https://containers.dev/features) - Node feature includes pnpm via Corepack |
| **Rationale** | Corepack is the Node.js-native package manager manager |
| **Alternatives Considered** | Separate pnpm feature from devcontainers-extra - adds unnecessary dependency |

**Recommended Configuration:**

```json
// .devcontainer/devcontainer.json
{
  "features": {
    "ghcr.io/devcontainers/features/node:1": {
      "version": "20"
    }
  },
  "postCreateCommand": "corepack enable && corepack prepare pnpm@latest-10 --activate"
}
```

**Alternative (dedicated pnpm feature):**

```json
{
  "features": {
    "ghcr.io/devcontainers-extra/features/pnpm:2": {
      "version": "10"
    }
  }
}
```

**Current Dev Container Status:**

The Radius dev container already includes Node.js. The update needed:

1. Add `corepack enable` to postCreateCommand
2. Add `corepack prepare pnpm@latest-10 --activate`

---

### 6. bicep-types npm Package Structure and Build Requirements

| Aspect | Details |
| ------ | ------- |
| **Decision** | The `src/bicep-types/` directory is a TypeScript package that REQUIRES local compilation |
| **Evidence** | Prototype validation - `npm install && npm run build` required in postinstall |
| **Rationale** | Package is TypeScript source; compiled `lib/` directory is not committed to git |
| **Alternatives Considered** | Wait for official npm publish - uncertain timeline; use as-is - doesn't work without build |

**Critical Finding:** The bicep-types package **cannot** be used directly from git without building. The compiled output (`lib/` directory) is `.gitignore`d.

**Package Structure:**

```text
src/bicep-types/
├── package.json          # Package configuration
├── tsconfig.json         # TypeScript compilation config
├── jest.config.ts        # Test configuration
├── .eslintrc.js          # Linting rules
├── README.md             # Package documentation
├── src/                  # TypeScript SOURCE (in git)
│   ├── index.ts          # Main exports
│   ├── types.ts          # Core type definitions
│   ├── indexer.ts        # Type indexing
│   ├── utils.ts          # Utilities
│   └── writers/
│       ├── json.ts       # JSON serialization
│       └── markdown.ts   # Markdown generation
├── lib/                  # ⚠️ COMPILED OUTPUT (NOT in git, must be built)
│   ├── index.js
│   ├── index.d.ts
│   └── ...
└── test/
    └── integration/      # Integration tests
```

**Build Process Required:**

```bash
# Inside src/bicep-types/ directory:
npm install    # Install devDependencies (typescript, etc.)
npm run build  # Compiles src/ → lib/
```

**Main Exports (after build):**

```typescript
// From lib/index.js (compiled from src/index.ts)
export * from "./writers/json";      // writeTypesJson, readTypesJson, writeIndexJson
export * from "./writers/markdown";  // writeMarkdown, writeIndexMarkdown
export * from "./indexer";           // buildIndex
export * from "./types";             // TypeFactory, TypeIndex, BicepType, etc.
```

**Key Types Used by Radius:**

| Type | Purpose |
| ---- | ------- |
| `TypeFactory` | Creates and manages Bicep types |
| `TypeIndex` | Index structure for resources and functions |
| `BicepType` | Union of all Bicep type variants |
| `ResourceType` | Resource type definition |
| `ObjectType` | Object type definition |
| `FunctionType` | Function type definition |

**Compatibility:** Standard TypeScript/npm package, compatible with pnpm after postinstall build.

---

## Implementation Decisions

### Package.json Updates

**Before (file: reference to submodule):**

```json
{
  "devDependencies": {
    "bicep-types": "file:../../../../bicep-types/src/bicep-types"
  }
}
```

**After (pnpm git reference with postinstall build - from prototype):**

```json
{
  "pnpm": {
    "onlyBuiltDependencies": ["autorest"]
  },
  "scripts": {
    "build": "tsc -p .",
    "test": "jest",
    "lint": "eslint src --ext ts",
    "lint:fix": "eslint src --ext ts --fix",
    "postinstall": "cd node_modules/bicep-types-repo/src/bicep-types && npm install && npm run build && cd ../../../.. && rm -rf node_modules/bicep-types && ln -sf bicep-types-repo/src/bicep-types node_modules/bicep-types"
  },
  "devDependencies": {
    "bicep-types-repo": "git+https://github.com/Azure/bicep-types.git#556bf5edad58e47ca57c6ddb1af155c3bcfdc5c7"
  }
}
```

**Key differences from original plan:**

- Package renamed to `bicep-types-repo` (pnpm fetches full repository to node_modules/)
- Uses `git+https://` URL format (not `github:` shorthand)
- Commit SHA appended with `#` (not combined with `&path:`)
- `postinstall` script handles: install → build → symlink
- `pnpm.onlyBuiltDependencies` allows autorest lifecycle scripts

### .npmrc Files (NEW)

Create `.npmrc` in both `autorest.bicep/` and `generator/` directories:

```properties
# Allow pnpm to install packages that need to run postinstall scripts
side-effects-cache = false
```

### Makefile Updates

**Before:**

```makefile
generate-bicep-types:
	git submodule update --init --recursive; \
	npm --prefix bicep-types/src/bicep-types install; \
	npm --prefix bicep-types/src/bicep-types ci && npm --prefix bicep-types/src/bicep-types run build; \
	npm --prefix hack/bicep-types-radius/src/autorest.bicep ci && ...
```

**After (from prototype):**

```makefile
.PHONY: generate-pnpm-installed
generate-pnpm-installed:
	@echo "$(ARROW) Detecting pnpm..."
	@which pnpm > /dev/null || { echo "pnpm is a required dependency. Run 'npm install -g pnpm' to install."; exit 1; }
	@echo "$(ARROW) OK"

.PHONY: generate-bicep-types
generate-bicep-types: generate-node-installed generate-pnpm-installed ## Generate Bicep extensibility types
	@echo "$(ARROW) Generating Bicep extensibility types from OpenAPI specs..."
	@echo "$(ARROW) Installing autorest.bicep dependencies (postinstall builds bicep-types)..."
	cd hack/bicep-types-radius/src/autorest.bicep && pnpm install
	@echo "$(ARROW) Building autorest.bicep..."
	pnpm --prefix hack/bicep-types-radius/src/autorest.bicep run build
	@echo "$(ARROW) Installing generator dependencies (postinstall builds bicep-types)..."
	cd hack/bicep-types-radius/src/generator && pnpm install
	@echo "$(ARROW) Running generator..."
	cd hack/bicep-types-radius/src/generator && pnpm run generate \
		--specs-dir ../../../../swagger --release-version ${VERSION} --verbose
```

**Key changes:**
- Added `generate-pnpm-installed` prerequisite target
- Uses `pnpm install` instead of `npm ci`
- Removed explicit bicep-types build steps (handled by postinstall)
- Removed `git submodule update --init --recursive`
- Uses `cd ... && pnpm install` pattern for cleaner execution

### Workflow Updates

**Before:**

```yaml
- uses: actions/checkout@v4
  with:
    submodules: recursive
```

**After:**

```yaml
- uses: actions/checkout@v4
  # No submodules property

- uses: pnpm/action-setup@v4
  with:
    version: 10

- uses: actions/setup-node@v4
  with:
    node-version-file: '.node-version'
    cache: 'pnpm'
```

### Dependabot Updates

**Remove:**

```yaml
- package-ecosystem: gitsubmodule
  directory: /
```

**Add (from prototype):**

```yaml
- package-ecosystem: npm
  directory: /hack/bicep-types-radius/src/autorest.bicep
  schedule:
    interval: weekly
  groups:
    autorest-bicep:
      patterns:
        - "*"

- package-ecosystem: npm
  directory: /hack/bicep-types-radius/src/generator
  schedule:
    interval: weekly
  groups:
    bicep-generator:
      patterns:
        - "*"
```

---

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
| ---- | ---------- | ------ | ---------- |
| ~~pnpm git subdirectory refs unstable~~ | N/A | N/A | ✅ Resolved: Using git+https:// reference + postinstall build instead |
| TypeScript build failure in postinstall | Low | High | Validated in prototype; postinstall is straightforward |
| Dependabot can't update git deps | Certain | Low | Document manual process; consider future scheduled workflow |
| Dev container pnpm issues | Low | Medium | Use Corepack which is Node.js-native |
| CI cache invalidation | Low | Low | Configure cache-dependency-path properly |
| Phantom dependency issues | Medium | Medium | pnpm's strictness catches issues early; fix during migration |
| Duplicate repo in node_modules | Certain | Low | Both packages fetch same repo independently; acceptable tradeoff |
| Postinstall script complexity | Low | Medium | Script is well-tested in prototype |

---

## Summary

| Research Question | Answer | Confidence |
| ----------------- | ------ | ---------- |
| Git subdirectory syntax | ❌ Does NOT work for TypeScript; use `git+https://` reference + postinstall build + symlink | ✅ High (prototype validated) |
| Git reference format | `git+https://github.com/Azure/bicep-types.git#<commit-sha>` | ✅ High (prototype validated) |
| TypeScript build strategy | `postinstall` script runs `npm install && npm run build` inside node_modules | ✅ High (prototype validated) |
| Symlink strategy | `ln -sf bicep-types-repo/src/bicep-types node_modules/bicep-types` | ✅ High (prototype validated) |
| Dependabot integration | `npm` ecosystem; git deps require manual updates | ✅ High |
| npm to pnpm migration | Fresh `pnpm install` for bicep-types packages; `pnpm import` for typespec | ✅ High |
| GitHub Actions setup | `pnpm/action-setup@v4` + `cache: 'pnpm'` | ✅ High |
| Dev container pnpm | Corepack activation in postCreateCommand | ✅ High |
| bicep-types package valid | Yes, but **requires local build** | ✅ High (prototype validated) |
| pnpm config required | `.npmrc` with `side-effects-cache = false` | ✅ High (prototype validated) |
| autorest lifecycle scripts | `pnpm.onlyBuiltDependencies: ["autorest"]` | ✅ High (prototype validated) |

**All research questions resolved. Prototype validated the approach. Plan 2 is ready for task breakdown.**
