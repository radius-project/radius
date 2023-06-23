## Accelerating verification on a k8s cluster

This is to help local development setup for quickly iterating on changes in the app core RP. 

Note, this only applies when we want to update the app core image, if we need to update the deployment/environment, a rerun of `rad init` may need to happen.

1. Create a new Azure Container Registry. [link to Azure Docs](https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-portal?tabs=azure-cli)

1. Set the environment variable DOCKER_REGISTRY set to the container registry
    ```
    export DOCKER_REGISTRY=<registry>.azurecr.io
    ```
1. Ensure you login to your container registry AND enable anonymous pull. The login command will need to be called every 3 hours as needed as it does log the user out frequently.
    ```
    az acr login -n <registry>
    az acr update --name <registry> --anonymous-pull-enabled
    ```
1. Build and push an initial version of all images in the radius repo. This should push images for the applications-rp, the ucpd, etc that are required to run radius.
    ```
    make docker-build && make docker-push
    ```
1. Deploy an environment with command on kubernetes
    ```
    go run ./cmd/rad/main.go install kubernetes --chart deploy/Chart --set rp.image=<registry>.azurecr.io/applications-rp,rp.tag=latest,ucp.image=<registry>.azurecr.io/ucpd,ucp.tag=latest
    go run ./cmd/rad/main.go workspace create kubernetes
    go run ./cmd/rad/main.go group create radius-rg
    go run ./cmd/rad/main.go switch radius-rg
    go run ./cmd/rad/main.go env create radius-rg --namespace default
    go run ./cmd/rad/main.go env switch radius-rg
    ```
1. Run a deployment. Executing `go run \cmd\rad\main.go deploy <bicep>` will deploy your file to the cluster.

## To deploy azure resources
The above steps will not configure the ability for radius to talk with azure resource. In order to do that, run everything above till Step 4 above and then:

1. Create Service Principal
    ```
    az ad sp create-for-rbac --role Owner --scope /subscriptions/<subscriptionId>/resourceGroups/<resourcegroupname>
    ```
    the output of the above command looks like the following
    ```
    {
     "appId": <appId>,
     "displayName": <displayNameValue>,
     "password": <pwd>,
     "tenant": <tenantId>
    }
    ```
1. Use these values in the following command:
    ```
    go run ./cmd/rad/main.go install kubernetes --chart deploy/Chart --set rp.image=<registry>.azurecr.io/applications-rp,rp.tag=latest,ucp.image=<registry>.azurecr.io/ucpd,ucp.tag=latest
    go run ./cmd/rad/main.go workspace create kubernetes
    go run ./cmd/rad/main.go group create radius-rg
    go run ./cmd/rad/main.go switch radius-rg
    go run ./cmd/rad/main.go env create radius-rg --namespace default
    go run ./cmd/rad/main.go env switch radius-rg
    go run ./cmd/rad/main.go env update radius-rg --azure-subscription-id <subscriptionId> --azure-resource-group <resourcegroupName>
    go run ./cmd/rad/main.go credential register azure --client-id <appId> --client-secret <pwd> --tenant-id <tenantId>
    ```
1. Run a deployment. Executing `go run \cmd\rad\main.go deploy <bicep>` will deploy your file to the cluster.

## To redeploy the applications resource provider
  
After validating the behavior with logs, commands, etc., you can quickly iterate on the applications resource provider with the following command
```
make docker-build-applications-rp && make docker-push-applications-rp && kubectl delete pod -l control-plane=applications-rp
```

This will:
- Build and push the applications-rp image
- Delete the running pod for the applications-rp. On restart, because we have specified the [latest tag](https://kubernetes.io/docs/concepts/containers/images/#updating-images) Kubernetes will repull the image each time
- The pod will restart with the latest image pushed prior.

## To redeploy the environment

Reploying an environment is a bit clunky right now. Use the following commands and deploy Radius again.

```
go run ./cmd/rad/main.go env delete <envname>
go run ./cmd/rad/main.go uninstall kubernetes
go run ./cmd/rad/main.go workspace delete
```

Another approach would be to delete and recreate the kubernetes cluster

Pro tip: `kubectl config set-context --current --namespace=radius-system` to the radius-system namespace
