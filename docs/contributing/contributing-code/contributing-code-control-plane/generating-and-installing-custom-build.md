# Generating a custom build

If you are working on changes to the Radius services, you can test the functionality end to end 
by creating a Radius installation with your code changes. For this, you need to:
	
- Choose a Kubernetes cluster to use for development.
- Build the Radius containers and push them to a container registry. 
- Install Radius from these images.

## Instructions

Prerequisite: [Supported Kubernetes cluster](https://docs.radapp.dev/operations/platforms/kubernetes-platform/supported-clusters/)

Note that installing a custom build could corrupt existing data or installation. For these reasons, it might be useful to install the custom build on a clean test cluster.  


1. Set environment variables for your docker registry and docker tag version. 
    ```
    export DOCKER_REGISTRY=your-registry.azurecr.io
    export DOCKER_IMAGE_TAG=latest
    ```

2. Log into your container registry and make sure you have permission to push images.

3. [Build Radius containers]( ../../contributing-code/contributing-code-building/README.md#building-containers) and push them to your registry. 
    ```
    make docker-build docker-push
    ```

4. Use the image from your container registry to install the Radius control plane.
    ```
    rad install kubernetes --chart deploy/Chart/ --set rp.image=your-registry.azurecr.io/appcore-rp,rp.tag=latest,ucp.image=your-registry.azurecr.io/ucpd,ucp.tag=latest
    ```

5. Use `rad init` command to set up the Radius workspace, environment, credentials, and provider as needed.
The setup is ready for use!

If you run into issues using working with a custom build, please refer to [Troubleshooting Installation](./troubleshooting-installation.md)