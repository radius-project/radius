# Applications.Core RP

> WIP - This doc will be updated iteratively.

## Layout

1. **/pkg/corerp/api**: API version specific models
2. **/pkg/corerp/datamodel**: API version agnostic models which is used for implmenting operation controller and storing resource metadata.
3. **/pkg/corerp/frontend/controller**: Per-operation controller implementations.
4. **/pkg/corerp/frontend/handler**: HTTP handler and routers.
5. **/pkg/corerp/frontend/middleware**: HTTP middleware.
6. **/pkg/corerp/frontend/hostingoptions**: The hosting options for service.
7. **/pkg/corerp/frontend/servicecontext**: The service context extracted from ARM proxy request header 

## Add new resource type and its controller

1. Define api version specific resource type models in [/pkg/corerp/api](api/) - create api-version directory if you need by referring the existing models.
2. Define api version agnostic datamodel in  [/pkg/corerp/datamodel](datamodel/) and its converters beteen datamodel and api models.
3. Define routes for new resource type and its operation APIs in [routes.go](frontend/handler/routes.go).
4. Create resource type directory under `/pkg/frontend/controller/` and related go files by referring to [environments controller](frontend/controller/environments/).
5. Create `handler_[resourcetype].go` by referring to [existing handler files](frontend/handler/).
6. Implement operation controllers and tests.

## References

* [ARM RPC v1.0 Specification](https://github.com/Azure/azure-resource-manager-rpc)