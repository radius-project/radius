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
1. Build and push an initial version of all images in the radius repo. This should push images for the appcore-rp, the ucpd, etc that are required to run radius.
    ```
    make docker-build && make docker-push
    ```
1. Deploy an environment with command on kubernetes
    ```
    go run .\cmd\rad\main.go env init kubernetes --chart deploy/Chart --appcore-image <registry>.azurecr.io/appcore-rp --appcore-tag latest --ucp-image <registry>.azurecr.io/ucpd --ucp-tag latest
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
    go run ./cmd/rad/main.go install kubernetes --chart ./deploy/Chart/ --force --provider-azure --provider-azure-resource-group <resourcegroupName> --provider-azure-subscription <subscriptionId> --provider-azure-client-id <appId> --provider-azure-client-secret <pwd> --provider-azure-tenant-id <tenantId> --appcore-image <registry>.azurecr.io/appcore-rp --appcore-tag latest
    ```
1. Run a deployment. Executing `go run \cmd\rad\main.go deploy <bicep>` will deploy your file to the cluster.

## To redeploy the appcore rp
  
After validating the behavior with logs, commands, etc., you can quickly iterate on the appcore rp with the following command
```
make docker-build-appcore-rp && make docker-push-appcore-rp && kubectl delete pod -l control-plane=appcore-rp
```

This will:
- Build and push the appcore-rp image
- Delete the running pod for the appcore-rp. On restart, because we have specified the [latest tag](https://kubernetes.io/docs/concepts/containers/images/#updating-images) Kubernetes will repull the image each time
- The pod will restart with the latest image pushed prior.

## To redeploy the environment

Reploying an environment is a bit clunky right now. This is the only consistent way I have done it:
  
```
go run .\cmd\rad\main.go env delete <envname>; go run .\cmd\rad\main.go env uninstall kubernetes; go run .\cmd\rad\main.go workspace delete <this is a command I added in my branch to delete the entry in config.yaml>; go run .\cmd\rad\main.go env init kubernetes --chart deploy/Chart --appcore-image <registry>.azurecr.io/appcore-rp --appcore-tag latest --ucp-image <registry>.azurecr.io/ucpd --ucp-tag latest
```
Another approach would be to delete and recreate the kubernetes cluster

Pro tip: `kubectl config set-context --current --namespace=radius-system` to the radius-system namespace
