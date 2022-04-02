# Deploy a custom Radius resource provider

> This guide will cover deploying a Radius environment to Azure with a custom build. This workflow is useful when you need to test changes to the templates, or when you need to override a setting that's not provided by the CLI.

### Step 1: (Optional) Build & Push the Image

You'll need a place to push Docker images that is publicly accessible. Alternatively you can use the public images from `radius.azurecr.io/radius-rp`.

```sh
docker build . -t <your registry>/radius-rp:latest
docker push <your registry>/radius-rp
```

### Step 2: Create an environment

**Deploy**

```sh
rad env init azure -i \
  --image <your registry>/radius-rp
```