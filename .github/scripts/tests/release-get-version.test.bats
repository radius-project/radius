#!/usr/bin/env bats

# filepath: /Users/yetkintimocin/dev/msft/radius-project/radius/.github/scripts/tests/release-get-version.test.bats

# Load the script to test
SCRIPT_PATH="../release-get-version.sh"

# Mock git ls-remote to avoid actual git calls
function git() {
  if [[ "$1" == "ls-remote" && "$2" == "--tags" ]]; then
    if [[ "$4" == "v0.45.0" ]]; then
      echo "ref/tags/v0.45.0"
      return 0
    fi
    return 1
  fi
}

# Helper function to run the script with arguments
function run_script() {
  # Save original GITHUB_OUTPUT and create a temp file
  local original_github_output="${GITHUB_OUTPUT:-}"
  export GITHUB_OUTPUT=$(mktemp)

  # Run the script with arguments
  source "$SCRIPT_PATH" "$1" "$2"

  # Get result from GITHUB_OUTPUT
  local result=$(cat "$GITHUB_OUTPUT")

  # Clean up and restore original GITHUB_OUTPUT
  rm -f "$GITHUB_OUTPUT"
  if [ -n "$original_github_output" ]; then
    export GITHUB_OUTPUT="$original_github_output"
  else
    unset GITHUB_OUTPUT
  fi

  echo "$result"
}

@test "Test standard version" {
  # Create a temporary directory to simulate a repository
  local temp_dir=$(mktemp -d)

  # Run the test
  result=$(run_script "v0.46.0" "$temp_dir")

  # Verify expected outputs
  echo "$result" | grep -q "release-version=v0.46.0"
  echo "$result" | grep -q "release-branch-name=release/0.46"
  echo "$result" | grep -q "release-channel=0.46"

  # Clean up
  rm -rf "$temp_dir"
}

@test "Test release candidate version" {
  # Create a temporary directory to simulate a repository
  local temp_dir=$(mktemp -d)

  # Run the test
  result=$(run_script "v0.46.0-rc2" "$temp_dir")

  # Verify expected outputs
  echo "$result" | grep -q "release-version=v0.46.0-rc2"
  echo "$result" | grep -q "release-branch-name=release/0.46"
  echo "$result" | grep -q "release-channel=0.46.0-rc2"

  # Clean up
  rm -rf "$temp_dir"
}

@test "Test existing tag" {
  # Create a temporary directory to simulate a repository
  local temp_dir=$(mktemp -d)

  # This should match our mocked git function that says v0.45.0 exists
  run bash -c "source $SCRIPT_PATH 'v0.45.0' '$temp_dir'"

  # Script should exit with error when no valid versions are found
  [ "$status" -eq 1 ]
  [[ "$output" =~ "No release version found" ]]

  # Clean up
  rm -rf "$temp_dir"
}

@test "Test multiple versions error" {
  # Create a temporary directory to simulate a repository
  local temp_dir=$(mktemp -d)

  # Run with multiple versions
  run source "$SCRIPT_PATH" "v0.46.0,v0.47.0" "$temp_dir"

  # Should fail with an error message about multiple versions
  [ "$status" -eq 1 ]
  [[ "$output" =~ "Updating multiple versions at once is not supported" ]]

  # Clean up
  rm -rf "$temp_dir"
}

@test "Test missing parameters" {
  # Test with no parameters
  run source "$SCRIPT_PATH"
  [ "$status" -eq 1 ]
  [[ "$output" =~ "VERSIONS is not set" ]]

  # Test with only one parameter
  run source "$SCRIPT_PATH" "v0.46.0"
  [ "$status" -eq 1 ]
  [[ "$output" =~ "REPOSITORY is not set" ]]
}
