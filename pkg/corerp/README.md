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
1. Create `handler_[resourcetype].go` by referring to [existing handler files](frontend/handler/).
1. Implement operation controllers and tests.

## References

* [ARM RPC v1.0 Specification](https://github.com/Azure/azure-resource-manager-rpc)