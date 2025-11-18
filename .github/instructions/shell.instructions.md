---
applyTo: "**/*.sh"
description: Instructions for shell script implementation using Bash conventions
---

# Shell Scripting Instructions

Instructions for writing clean, safe, and maintainable shell scripts for bash, sh, zsh, and other shells.

## General Principles

- Generate code that is clean, simple, and concise
- Ensure scripts are easily readable and understandable
- Keep comments to a minimum, only add comments when the logic may be confusing (regex, complex if statements, confusing variables, etc.)
- Generate concise and simple echo outputs to provide execution status
- Avoid unnecessary echo output and excessive logging
- Always strive to keep bash and shell code simple and well crafted
- Only add functions when needed to simplify logic
- Use shellcheck for static analysis when available
- Assume scripts are for automation and testing rather than production systems unless specified otherwise
- Use the correct and latest conventions for Bash, version 5.3 release
- Prefer safe expansions: double-quote variable references (`"$var"`), use `${var}` for clarity, and avoid `eval`
- Use modern Bash features (`[[ ]]`, `local`, arrays) when portability requirements allow; fall back to POSIX constructs only when needed
- Choose reliable parsers for structured data instead of ad-hoc text processing

## Error Handling & Safety

### Core Safety Principles

- Always enable `set -euo pipefail` to fail fast on errors, catch unset variables, and surface pipeline failures
- Validate all required parameters before execution
- Provide clear error messages with context
- Use `trap` to clean up temporary resources or handle unexpected exits when the script terminates
- Declare immutable values with `readonly` (or `declare -r`) to prevent accidental reassignment
- Use `mktemp` to create temporary files or directories safely and ensure they are removed in your cleanup handler

### Error Handling and Output Guidelines

When it's obvious errors should be visible or users explicitly request that errors be visible:

- NEVER redirect stderr to `/dev/null` or suppress error output
- Allow commands to fail naturally and show their native error messages
- Use `|| echo ""` pattern only when you need to capture output but allow graceful failure
- Prefer natural bash error propagation over complex error capture and re-display
- Let tools like `az login` fail naturally with their built-in error messages

## Script Structure

- Start with a clear shebang: `#!/bin/bash` unless specified otherwise
- Include a header comment explaining the script's purpose
- Define default values for all variables at the top
- Use functions for reusable code blocks
- Create reusable functions instead of repeating similar blocks of code
- Keep the main execution flow clean and readable

### Script Template Example

```bash
#!/bin/bash

# ============================================================================
# Script Description Here
# ============================================================================

set -euo pipefail

cleanup() {
  # Remove temporary resources or perform other teardown steps as needed
  if [[ -n "${TEMP_DIR:-}" && -d "${TEMP_DIR}" ]]; then
    rm -rf "${TEMP_DIR}"
  fi
}

trap cleanup EXIT

# Default values
RESOURCE_GROUP=""
REQUIRED_PARAM=""
OPTIONAL_PARAM="default-value"
readonly SCRIPT_NAME="$(basename "$0")"

TEMP_DIR=""

# Functions
usage() {
  echo "Usage: ${SCRIPT_NAME} [OPTIONS]"
  echo "Options:"
  echo "  -g, --resource-group   Resource group (required)"
  echo "  -h, --help            Show this help"
  exit 0
}

validate_requirements() {
  if [[ -z "${RESOURCE_GROUP}" ]]; then
    echo "Error: Resource group is required"
    exit 1
  fi
}

main() {
  validate_requirements

  TEMP_DIR="$(mktemp -d)"
  if [[ ! -d "${TEMP_DIR}" ]]; then
    echo "Error: failed to create temporary directory" >&2
    exit 1
  fi

  echo "============================================================================"
  echo "Script Execution Started"
  echo "============================================================================"

  # Main logic here

  echo "============================================================================"
  echo "Script Execution Completed"
  echo "============================================================================"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    -g | --resource-group)
      RESOURCE_GROUP="$2"
      shift 2
      ;;
    -h | --help)
      usage
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

# Execute main function
main "$@"
```

## Working with JSON and YAML

- Prefer dedicated parsers (`jq` for JSON, `yq` for YAML - or `jq` on JSON converted via `yq`) over ad-hoc text processing with `grep`, `awk`, or shell string splitting
- When `jq`/`yq` are unavailable or not appropriate, choose the next most reliable parser available in your environment, and be explicit about how it should be used safely
- Validate that required fields exist and handle missing/invalid data paths explicitly (e.g., by checking `jq` exit status or using `// empty`)
- Quote jq/yq filters to prevent shell expansion and prefer `--raw-output` when you need plain strings
- Treat parser errors as fatal: combine with `set -euo pipefail` or test command success before using results
- Document parser dependencies at the top of the script and fail fast with a helpful message if `jq`/`yq` (or alternative tools) are required but not installed

## Azure CLI and JMESPath Guidelines

When working with Azure CLI:

- Research Azure CLI command output structure using #fetch:<https://docs.microsoft.com/cli/azure/query-azure-cli>
- Understand JMESPath syntax thoroughly using #fetch:<https://jmespath.org/tutorial.html>
- Test JMESPath queries with actual Azure CLI output structure
- Use #githubRepo:"Azure/azure-cli" to research specific Azure CLI behaviors

## ShellCheck and Formatting Compliance

You will always follow all shellcheck and shfmt formatting rules:

- ALWAYS use the get_errors #problems tool to check for linting issues in shell files you are working on
- Use `shellcheck` command line tool only when get_errors #problems tool is not available
- Always follow Shell Style Guide formatting rules from #fetch:<https://google.github.io/styleguide/shellguide.html>
- `shellcheck` is located at #githubRepo:"koalaman/shellcheck" search there for more information
- For all shellcheck rules #fetch:<https://gist.githubusercontent.com/nicerobot/53cee11ee0abbdc997661e65b348f375/raw/d5a97b3b18ead38f323593532050f0711084acf1/_shellcheck.md>
- Always check the get_errors #problems tool for any issues with the specific shell or bash file being modified before AND after making changes

### Formatting Rules (Shell Style Guide)

Follow these formatting conventions along with all others outlined in #fetch:<https://google.github.io/styleguide/shellguide.html>:

- Use 4 spaces for indentation, never tabs
- Maximum line length is 80 characters
- Put `; then` and `; do` on the same line as `if`, `for`, `while`
- Use `[[ ... ]]` instead of `[ ... ]` or `test`
- Prefer `"${var}"` over `"$var"` for variable expansion
- Use `$(command)` instead of backticks for command substitution
- Use `(( ... ))` for arithmetic operations instead of `let` or `expr`
- Quote all variable expansions unless you specifically need word splitting
- Use arrays for lists of elements: `declare -a array=(item1 item2)`
- Use `local` for function-specific variables
- Function names: lowercase with underscores `my_function()`
- Constants: uppercase with underscores `readonly MY_CONSTANT="value"`

## Research and Validation Requirements

Before making changes to shell scripts:

- Use #fetch to research Azure CLI documentation when working with `az` commands
- Use #githubRepo to research Azure CLI source code for complex scenarios
- Always validate your understanding of command output formats
- Test complex logic patterns and JMESPath queries before implementation
- Understand the user's specific requirements about error handling and output visibility

---

<!-- End of Shell Scripting Instructions -->
