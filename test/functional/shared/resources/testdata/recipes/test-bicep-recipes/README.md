# Test Recipes

The recipes in this folder are published as part of the PR process to:

> `radiusdev.azurecr.io/test/functional/shared/recipes/<filename>:pr-<pr-number>`

This is important because it allows us to make changes to the recipes, and test them in the same PR that contains the change.

## Non-recipes bicep files

Any Bicep file starting with `_` will be skipped during publishing. Use this as a convention to create shared modules that are not published as recipes. For example `_redis_kubernetes.bicep` would not be published.