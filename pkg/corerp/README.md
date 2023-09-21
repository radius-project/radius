# Applications.Core RP

> WIP - This doc will be updated iteratively.

## Layout

1. **/pkg/corerp/api**: API version specific models
1. **/pkg/corerp/datamodel**: API version agnostic models to implement operation controller and store resource metadata.
1. **/pkg/corerp/frontend/controller**: Per-operation controller implementations.
1. **/pkg/corerp/frontend/handler**: HTTP server handler and routers.
1. **/pkg/corerp/frontend/middleware**: HTTP server middleware.
1. **/pkg/corerp/frontend/hostingoptions**: Hosting options for resource provider service.
1. **/pkg/corerp/frontend/servicecontext**: Service context extracted from ARM proxy request header.

## Add new resource type and its controller

1. Ensure that you update openapi spec in [/swagger](../../swagger)
1. Generate resource type models in [/pkg/corerp/api](api/) by following the instruction.
1. Define api version agnostic datamodel in  [/pkg/corerp/datamodel](datamodel/) and its converters beteen datamodel and api models.
1. Define routes for new resource type and its operation APIs in [routes.go](frontend/handler/routes.go).
1. Create resource type directory under `/pkg/frontend/controller/` and related go files by referring to [environments controller](frontend/controller/environments/).
1. Implement operation controllers and tests.
1. Register handlers in [handlers.go](frontend/handler/handlers.go).

## How to Run and Test Core RP

1. Update StorageProvider section of `cmd/applications-rp/radius-dev.yaml` by adding your Cosmos DB URL and key
1. With `cmd/applications-rp/main.go` file open, go to `Run And Debug` view in VS Code and click `Run`
1. You should have the service up and running at `localhost:8080` now
1. To create or update an environment, here is an example curl command:

    ```
    curl --location --request PUT 'http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0?api-version=2023-10-01-preview' \
    --header 'X-Ms-Arm-Resource-System-Data: {"lastModifiedBy":"fake@hotmail.com","lastModifiedByType":"User","lastModifiedAt":"2022-03-22T18:54:52.6857175Z"}' \
    --header 'Content-Type: application/json' \
    --data-raw '{
        "properties": {
            "compute": {
                "kind": "Kubernetes",
                "resourceId": "test-override-2"
            }
        }
    }'
    ```

1. To get information about an environment, here is an example curl command:

    ```
    curl --location --request GET 'http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/?api-version=2023-10-01-preview'
    ```

1. You should also be able to see all changes in Cosmos DB

## References

* [ARM RPC v1.0 Specification](https://github.com/Azure/azure-resource-manager-rpc)
