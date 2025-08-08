# Generating a custom build

If you are working on changes to the Radius services, you can test the functionality end to end
by creating a Radius installation with your code changes. For this, you need to:

- Choose a Kubernetes cluster to use for development.
- Build the Radius containers and push them to a container registry.
- Install Radius from these images.

## Instructions

Prerequisite: [Supported Kubernetes cluster](https://docs.radapp.io/guides/operations/kubernetes/overview)

Note that installing a custom build could corrupt existing data or installation. For these reasons, it might be useful to install the custom build on a clean test cluster.

1. Set environment variables for your docker registry and docker tag version.

   ```console
   export DOCKER_REGISTRY=ghcr.io/your-registry
   export DOCKER_IMAGE_TAG=latest
   ```

2. Log into your container registry and make sure you have permission to push images.

3. [Build Radius containers](../../contributing-code/contributing-code-building/README.md#building-containers) and push them to your registry.

   ```console
   make docker-build docker-push
   ```

4. Use the image from your container registry to install the Radius control plane.

   ```console
   rad install kubernetes --chart deploy/Chart/ --set global.imageRegistry=ghcr.io/your-registry --set global.imageTag=latest
   ```

   For private registries that require authentication, first create a Kubernetes secret:

   ```console
   kubectl create secret docker-registry regcred \
     --docker-server=ghcr.io/your-registry \
     --docker-username=<username> \
     --docker-password=<password> \
     -n radius-system

   rad install kubernetes --chart deploy/Chart/ \
     --set global.imageRegistry=ghcr.io/your-registry \
     --set global.imageTag=latest \
     --set-string 'global.imagePullSecrets[0].name=regcred'
   ```

5. Use `rad init` command to set up the Radius workspace, environment, credentials, and provider as needed.
   The setup is ready for use!

If you run into issues using working with a custom build, please refer to [Troubleshooting Installation](./troubleshooting-installation.md)
