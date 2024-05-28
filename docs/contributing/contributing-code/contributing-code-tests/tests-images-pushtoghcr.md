
## Using Standard Images in Tests

When writing tests for our applications, it's common to use standard images for resources like Redis, Mongo, and others. By default, specifying an image with `redis:latest` or similar tags will pull the image from Docker Hub. This behavior is due to Docker's default configuration, which automatically searches the Docker Hub registry for images when no other registry is specified. 

 We are transitioning to using images hosted in our public GitHub Container Registry (GHCR) repository. This change ensures more reliable access to the images needed for our tests and will help reduce potential issues caused by external dependencies.

### New Image Pull Guideline

Moving forward, we will pull standard test images from our public GHCR repository. 

For example, moving forward, we will use:

```yaml
image: ghcr.io/radius-project/mirror/redis:latest
```

This change applies to all standard images. Please update any new tests accordingly.

### Adding Images to GHCR Repository

To add new images to the GHCR repository, you can follow these steps:

Our test pipelines operate on machines that utilize the AMD64 architecture. We need to pull the image from Docker that is compatible with this platform architecture.
For example:

Use the following command to pull an image for the AMD64 architecture:

```bash
docker pull --platform linux/amd64 redis:latest
```

After pulling the image, tag it with the GHCR path for Radius project:
```bash
docker tag redis:latest ghcr.io/radius-project/mirror/redis:latest
```

Push the image to the GHCR repository:
```bash
docker push ghcr.io/radius-project/mirror/redis:latest
```

This will upload the image to the Radius GHCR public repository, making it available for use.


### Setting Permissions for Images in GHCR

Once you have pushed images to the GHCR and tested them locally, it's important to set the appropriate permissions to ensure security and proper access control. By default, new images pushed to the GHCR are set to private. To make an image publicly readable or to restrict access, you will need to update permissions.

### Making Images Read-Only

To prevent unauthorized modifications, please set images to be read-only for everyone except maintainers, who will have admin permissions. This ensures that only authorized personnel can update or delete images.

### Steps to Update Image Permissions

1. Navigate to the GHCR page for the image. [Search Page](https://github.com/orgs/radius-project/packages)
2. Click on the "Package settings" wheel for the image.
3. In the "Manage access" section, add relevant repositories, codespace repositories and set their access level to Read.
4. Set image to be publicly accessible.
5. Grant "Admin" role to `maintainers-radius`.

#### Note: Adding a New Label to an existing image in ghcr.
If you need to add a new label to an image that's already hosted in the Radius GHCR repository [link](ghcr.io/radius-project/mirror/), please reach out to the maintainers-radius team to obtain `write` access permissions for the image repository.