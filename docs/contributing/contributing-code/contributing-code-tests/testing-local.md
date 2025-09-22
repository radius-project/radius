# Accelerating verification on a k8s cluster

This is to help local development setup for quickly iterating on changes in the app core RP.

Note, this only applies when we want to update the app core image, if we need to update the deployment/environment, a rerun of `rad init` may need to happen.

1. Create a new Azure Container Registry. [link to Azure Docs](https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-portal?tabs=azure-cli)

1. Set the environment variable DOCKER_REGISTRY set to the container registry

   ```console
   export DOCKER_REGISTRY=ghcr.io/your-registry
   ```

1. Ensure you login to your container registry. The login command will need to be called every 3 hours as needed as it does log the user out frequently.

   ```console
   az acr login -n <registry>
   ```

   **Security Note**: For production environments, anonymous pull should be disabled for security reasons. Anonymous pull is disabled by default on ACR. To explicitly disable it (recommended for production):

   ```console
   az acr update --name <registry> --anonymous-pull-enabled false
   ```

   For local development and testing only, you may optionally enable anonymous pull to simplify image access:

   ```console
   az acr update --name <registry> --anonymous-pull-enabled true
   ```

   **Warning**: Only enable anonymous pull for development/testing registries. Never enable it for production registries containing sensitive or proprietary images.

1. Build and push an initial version of all images in the Radius repo. This should push images for the applications-rp, the ucpd, etc that are required to run radius.

   ```console
   make docker-build && make docker-push
   ```

1. Deploy an environment with command on kubernetes

   ```console
   go run ./cmd/rad/main.go install kubernetes --chart deploy/Chart --set global.imageRegistry=ghcr.io/your-registry --set global.imageTag=latest
   go run ./cmd/rad/main.go workspace create kubernetes
   go run ./cmd/rad/main.go group create radius-rg
   go run ./cmd/rad/main.go switch radius-rg
   go run ./cmd/rad/main.go env create radius-rg --namespace default
   go run ./cmd/rad/main.go env switch radius-rg
   ```

   Note: If your registry requires authentication, create a Kubernetes secret and use imagePullSecrets:

   ```console
   kubectl create secret docker-registry regcred \
     --docker-server=ghcr.io/your-registry \
     --docker-username=<username> \
     --docker-password=<password> \
     -n radius-system

   go run ./cmd/rad/main.go install kubernetes --chart deploy/Chart \
     --set global.imageRegistry=ghcr.io/your-registry \
     --set global.imageTag=latest \
     --set-string 'global.imagePullSecrets[0].name=regcred'
   ```

1. Run a deployment. Executing `go run \cmd\rad\main.go deploy <bicep>` will deploy your file to the cluster.

## To deploy azure resources

The above steps will not configure the ability for Radius to talk with azure resource. In order to do that, run everything above till Step 4 above and then:

1. Create Service Principal

   ```console
   az ad sp create-for-rbac --role Owner --scope /subscriptions/<subscriptionId>/resourceGroups/<resourcegroupname>
   ```

   the output of the above command looks like the following

   ```json
   {
    "appId": <appId>,
    "displayName": <displayNameValue>,
    "password": <pwd>,
    "tenant": <tenantId>
   }
   ```

1. Use these values in the following command:

   ```console
   go run ./cmd/rad/main.go install kubernetes --chart deploy/Chart --set global.imageRegistry=ghcr.io/your-registry --set global.imageTag=latest
   go run ./cmd/rad/main.go workspace create kubernetes
   go run ./cmd/rad/main.go group create radius-rg
   go run ./cmd/rad/main.go switch radius-rg
   go run ./cmd/rad/main.go env create radius-rg --namespace default
   go run ./cmd/rad/main.go env switch radius-rg
   go run ./cmd/rad/main.go env update radius-rg --azure-subscription-id <subscriptionId> --azure-resource-group <resourcegroupName>
   go run ./cmd/rad/main.go credential register azure sp --client-id <appId> --client-secret <pwd> --tenant-id <tenantId>
   ```

1. Run a deployment. Executing `go run \cmd\rad\main.go deploy <bicep>` will deploy your file to the cluster.

## To redeploy the applications resource provider

After validating the behavior with logs, commands, etc., you can quickly iterate on the applications resource provider with the following command

```console
make docker-build-applications-rp && make docker-push-applications-rp && kubectl delete pod -l control-plane=applications-rp
```

This will:

- Build and push the applications-rp image
- Delete the running pod for the applications-rp. On restart, because we have specified the [latest tag](https://kubernetes.io/docs/concepts/containers/images/#updating-images) Kubernetes will repull the image each time
- The pod will restart with the latest image pushed prior.

## To redeploy the environment

Reploying an environment is a bit clunky right now. Use the following commands and deploy Radius again.

```console
go run ./cmd/rad/main.go env delete <envname>
go run ./cmd/rad/main.go uninstall kubernetes
go run ./cmd/rad/main.go workspace delete
```

Another approach would be to delete and recreate the kubernetes cluster

Pro tip: `kubectl config set-context --current --namespace=radius-system` to the radius-system namespace
