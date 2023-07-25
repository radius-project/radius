# Test Terraform Recipes

The recipes in this folder are published as part of the PR process to a web server running inside the Kubernetes cluster.

This is important because it allows us to make changes to the recipes, and test them in the same PR that contains the change.

## Structure

Each directory should contain a Terraform module. The module will be zipped and uploaded to the cluster. Then it will be accessible to terraform using a URL like:

```txt
http://tf-module-server.radius-test-tf-module-server.svc.cluster.local/<recipe_name>.zip
```

This can be used in [Terraform](https://developer.hashicorp.com/terraform/language/modules/sources#fetching-archives-over-http) as the `source` attribute of a module.

This URL is only accessible inside the cluster.

## Testing

If you need to verify the content, use:

```txt
kubectl port-forward svc/tf-module-server 8080:80 -n radius-test-tf-module-server 
```

Then you can access the recipe download at:

```txt
http://localhost:8080/<recipe_name>.zip
```

---- 

Or reference a module from Terraform:

```hcl
module "testing" {
  source = "http://localhost:8080/<recipe_name>.zip"
}
```