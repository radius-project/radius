#! /bin/bash

# Fail immedietly if any command fails
set -e

# Get command line arguments
BICEP_PATH=$1
DIRECTORY=$2
REGISTRY_PATH=$3
RECIPE_VERSION=$4

BICEP_EXECUTABLE="$BICEP_PATH/rad-bicep"

# Print usage information
function print_usage() {
    echo "Usage: $0 <BICEP_PATH> <DIRECTORY> <REGISTRY_PATH> <RECIPE_VERSION>"
    echo ""
    echo "  Publishes all recipes in the repository to the Azure Container Registry. Requires you to be logged into Azure via az login."
    echo ""
    echo "  BICEP_PATH: Path to directory containing the bicep executable. For example, ~/.rad/bin"
    echo "  DIRECTORY: Directory containing the recipes to publish. For example, ./test/functional/testdata/recipes"
    echo "  REGISTRY_PATH: Registry hostname and path prefix. For example, myregistry.azurecr.io/tests/recipes."
    echo "  RECIPE_VERSION: Version of the recipe to publish. For example, pr-19293"
    echo ""
}

# Verify that the required arguments are present
if [[ $# -ne 4 ]]; then
    echo "Error: Missing required arguments"
    echo ""
    print_usage
    exit 1
fi

# We create output that's intended to be consumed by the GitHub Action summary. If we're
# not running in a GitHub Action, we'll just silence the output.
if [[ -z "$GITHUB_STEP_SUMMARY" ]]; then
    GITHUB_STEP_SUMMARY=/dev/null
fi

echo "## Recipes published to $REGISTRY_PATH" >> $GITHUB_STEP_SUMMARY
for RECIPE in $(find "$DIRECTORY" -type f -name "*.bicep")
do
    FILENAME=$(basename $RECIPE)
    PUBLISH_REF="$REGISTRY_PATH/${FILENAME%.*}:$RECIPE_VERSION"
    
    # Skip files that start with _. These are not recipes, they are modules that are
    # used by the recipes.
    if [[ $RECIPE = _* ]]; then
        echo "Skipping $RECIPE"
        continue
    fi

    echo "Publishing $RECIPE to $PUBLISH_REF"
    echo "- $PUBLISH_REF" >> $GITHUB_STEP_SUMMARY
    $BICEP_EXECUTABLE publish $RECIPE --target "br:$PUBLISH_REF"
done
