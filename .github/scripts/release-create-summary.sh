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