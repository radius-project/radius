/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package radius

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	planes_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/planes"
	radius_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/radius"
	resourcegroups_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/resourcegroups"
	resourceproviders_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/resourceproviders"
	"github.com/radius-project/radius/pkg/validator"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	// OperationTypeUCPRadiusProxy is the operation type for proxying Radius API calls.
	OperationTypeUCPRadiusProxy = "UCPRADIUSPROXY"

	// operationRetryAfter tells clients to poll in 1 second intervals. Our operations are fast.
	operationRetryAfter = time.Second * 1
)

func (m *Module) Initialize(ctx context.Context) (http.Handler, error) {
	apiValidator := validator.APIValidator(validator.Options{
		SpecLoader:         m.options.SpecLoader,
		ResourceTypeGetter: validator.UCPResourceTypeGetter,
	})

	transport := otelhttp.NewTransport(http.DefaultTransport)

	// More convienent way to capture errors
	var err error
	capture := func(handler http.HandlerFunc, e error) http.HandlerFunc {
		if e != nil {
			err = errors.Join(err, e)
		}

		return handler
	}

	databaseClient, err := m.options.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	ctrlOptions := controller.Options{
		Address:        m.options.Config.Server.Address(),
		DatabaseClient: databaseClient,
		PathBase:       m.options.Config.Server.PathBase,
		StatusManager:  m.options.StatusManager,

		KubeClient:   nil, // Unused by Radius module
		ResourceType: "",  // Set dynamically
	}

	// NOTE: we're careful where we use the `apiValidator` middleware. It's not used for the proxy routes.
	m.router.Route(m.options.Config.Server.PathBase+"/planes/radius", func(r chi.Router) {
		r.With(apiValidator).Get("/", capture(radiusPlaneListHandler(ctx, ctrlOptions)))
		r.Route("/{planeName}", func(r chi.Router) {
			r.With(apiValidator).Get("/", capture(radiusPlaneGetHandler(ctx, ctrlOptions)))
			r.With(apiValidator).Put("/", capture(radiusPlanePutHandler(ctx, ctrlOptions)))
			r.With(apiValidator).Delete("/", capture(radiusPlaneDeleteHandler(ctx, ctrlOptions)))

			r.Route("/providers", func(r chi.Router) {
				r.Get("/", capture(resourceProviderSummaryListHandler(ctx, ctrlOptions)))
				r.Get("/{resourceProviderName}", capture(resourceProviderSummaryGetHandler(ctx, ctrlOptions)))

				r.Route("/System.Resources", func(r chi.Router) {

					// Routes for async support: operationResults + operationStatuses
					r.Route("/locations/{location}", func(r chi.Router) {
						r.Get("/operationStatuses/{operationId}", capture(operationStatusGetHandler(ctx, ctrlOptions)))
						r.Get("/operationResults/{operationId}", capture(operationResultGetHandler(ctx, ctrlOptions)))
					})

					r.Route("/resourceproviders", func(r chi.Router) {
						r.With(apiValidator).Get("/", capture(resourceProviderListHandler(ctx, ctrlOptions)))
						r.Route("/{resourceProviderName}", func(r chi.Router) {
							r.With(apiValidator).Get("/", capture(resourceProviderGetHandler(ctx, ctrlOptions)))
							r.With(apiValidator).Put("/", capture(resourceProviderPutHandler(ctx, ctrlOptions)))
							r.With(apiValidator).Delete("/", capture(resourceProviderDeleteHandler(ctx, ctrlOptions)))

							r.Route("/locations", func(r chi.Router) {
								r.With(apiValidator).Get("/", capture(locationListHandler(ctx, ctrlOptions)))
								r.Route("/{locationName}", func(r chi.Router) {
									r.With(apiValidator).Get("/", capture(locationGetHandler(ctx, ctrlOptions)))
									r.With(apiValidator).Put("/", capture(locationPutHandler(ctx, ctrlOptions)))
									r.With(apiValidator).Delete("/", capture(locationDeleteHandler(ctx, ctrlOptions)))
								})
							})

							r.Route("/resourcetypes", func(r chi.Router) {
								r.With(apiValidator).Get("/", capture(resourceTypeListHandler(ctx, ctrlOptions)))
								r.Route("/{resourceTypeName}", func(r chi.Router) {
									r.With(apiValidator).Get("/", capture(resourceTypeGetHandler(ctx, ctrlOptions)))
									r.With(apiValidator).Put("/", capture(resourceTypePutHandler(ctx, ctrlOptions)))
									r.With(apiValidator).Delete("/", capture(resourceTypeDeleteHandler(ctx, ctrlOptions)))

									r.Route("/apiversions", func(r chi.Router) {
										r.With(apiValidator).Get("/", capture(apiVersionListHandler(ctx, ctrlOptions)))
										r.Route("/{apiVersionName}", func(r chi.Router) {
											r.With(apiValidator).Get("/", capture(apiVersionGetHandler(ctx, ctrlOptions)))
											r.With(apiValidator).Put("/", capture(apiVersionPutHandler(ctx, ctrlOptions)))
											r.With(apiValidator).Delete("/", capture(apiVersionDeleteHandler(ctx, ctrlOptions)))
										})
									})
								})
							})
						})
					})
				})

				// Proxy to plane-scoped ResourceProvider APIs
				//
				// NOTE: DO NOT validate schema for proxy routes.
				r.Handle("/*", capture(planeScopedProxyHandler(ctx, ctrlOptions, transport, m.defaultDownstream)))
			})

			r.Route("/resourcegroups", func(r chi.Router) {
				r.With(apiValidator).Get("/", capture(resourceGroupListHandler(ctx, ctrlOptions)))
				r.Route("/{resourceGroupName}", func(r chi.Router) {
					r.With(apiValidator).Get("/", capture(resourceGroupGetHandler(ctx, ctrlOptions)))
					r.With(apiValidator).Put("/", capture(resourceGroupPutHandler(ctx, ctrlOptions)))
					r.With(apiValidator).Delete("/", capture(resourceGroupDeleteHandler(ctx, ctrlOptions)))
					r.With(apiValidator).Route("/resources", func(r chi.Router) {
						r.Get("/", capture(resourceGroupResourcesHandler(ctx, ctrlOptions)))
					})

					r.Route("/providers", func(r chi.Router) {
						// Proxy to resource-group-scoped ResourceProvider APIs
						//
						// NOTE: DO NOT validate schema for proxy routes.
						r.Handle("/*", capture(resourceGroupScopedProxyHandler(ctx, ctrlOptions, transport, m.defaultDownstream)))
					})
				})

			})
		})
	})

	return m.router, nil
}

var planeResourceOptions = controller.ResourceOptions[datamodel.RadiusPlane]{
	RequestConverter:         converter.RadiusPlaneDataModelFromVersioned,
	ResponseConverter:        converter.RadiusPlaneDataModelToVersioned,
	AsyncOperationRetryAfter: operationRetryAfter,
}

var planeResourceType = "System.Radius/planes"

func radiusPlaneListHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, planeResourceType, v1.OperationList, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return &planes_ctrl.ListPlanesByType[*datamodel.RadiusPlane, datamodel.RadiusPlane]{
			Operation: controller.NewOperation(opts, planeResourceOptions),
		}, nil
	})
}

func radiusPlaneGetHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, planeResourceType, v1.OperationGet, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewGetResource(opts, planeResourceOptions)
	})
}

func radiusPlanePutHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, planeResourceType, v1.OperationPut, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultSyncPut(opts, planeResourceOptions)
	})
}

func radiusPlaneDeleteHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, planeResourceType, v1.OperationDelete, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultSyncDelete(opts, planeResourceOptions)
	})
}

var resourceGroupResourceOptions = controller.ResourceOptions[datamodel.ResourceGroup]{
	RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
	ResponseConverter: converter.ResourceGroupDataModelToVersioned,
}

func resourceGroupListHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, v20231001preview.ResourceGroupType, v1.OperationList, ctrlOptions, resourcegroups_ctrl.NewListResourceGroups)
}

func resourceGroupGetHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, v20231001preview.ResourceGroupType, v1.OperationGet, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewGetResource(opts, resourceGroupResourceOptions)
	})
}

func resourceGroupPutHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, v20231001preview.ResourceGroupType, v1.OperationPut, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultSyncPut(opts, resourceGroupResourceOptions)
	})
}

func resourceGroupDeleteHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, v20231001preview.ResourceGroupType, v1.OperationDelete, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultSyncDelete(opts, resourceGroupResourceOptions)
	})
}

func resourceGroupResourcesHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, v20231001preview.ResourceType, v1.OperationList, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return resourcegroups_ctrl.NewListResources(opts)
	})
}

func resourceProviderSummaryListHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.ResourceProviderSummaryResourceType, v1.OperationList, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return resourceproviders_ctrl.NewListResourceProviderSummaries(opts)
	})
}

func resourceProviderSummaryGetHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.ResourceProviderSummaryResourceType, v1.OperationGet, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return resourceproviders_ctrl.NewGetResourceProviderSummary(opts)
	})
}

var resourceProviderResourceOptions = controller.ResourceOptions[datamodel.ResourceProvider]{
	RequestConverter:         converter.ResourceProviderDataModelFromVersioned,
	ResponseConverter:        converter.ResourceProviderDataModelToVersioned,
	AsyncOperationRetryAfter: operationRetryAfter,
}

func resourceProviderListHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.ResourceProviderResourceType, v1.OperationList, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewListResources(opts, resourceProviderResourceOptions)
	})
}

func resourceProviderGetHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.ResourceProviderResourceType, v1.OperationGet, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewGetResource(opts, resourceProviderResourceOptions)
	})
}

func resourceProviderPutHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.ResourceProviderResourceType, v1.OperationPut, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultAsyncPut(opts, resourceProviderResourceOptions)
	})
}

func resourceProviderDeleteHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.ResourceProviderResourceType, v1.OperationDelete, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultAsyncDelete(opts, resourceProviderResourceOptions)
	})
}

var resourceTypeResourceOptions = controller.ResourceOptions[datamodel.ResourceType]{
	RequestConverter:         converter.ResourceTypeDataModelFromVersioned,
	ResponseConverter:        converter.ResourceTypeDataModelToVersioned,
	AsyncOperationRetryAfter: operationRetryAfter,
}

func resourceTypeListHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.ResourceTypeResourceType, v1.OperationList, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewListResources(opts, resourceTypeResourceOptions)
	})
}

func resourceTypeGetHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.ResourceTypeResourceType, v1.OperationGet, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewGetResource(opts, resourceTypeResourceOptions)
	})
}

func resourceTypePutHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.ResourceTypeResourceType, v1.OperationPut, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultAsyncPut(opts, resourceTypeResourceOptions)
	})
}

func resourceTypeDeleteHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.ResourceTypeResourceType, v1.OperationDelete, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultAsyncDelete(opts, resourceTypeResourceOptions)
	})
}

var apiVersionResourceOptions = controller.ResourceOptions[datamodel.APIVersion]{
	RequestConverter:         converter.APIVersionDataModelFromVersioned,
	ResponseConverter:        converter.APIVersionDataModelToVersioned,
	AsyncOperationRetryAfter: operationRetryAfter,
}

func apiVersionListHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.APIVersionResourceType, v1.OperationList, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewListResources(opts, apiVersionResourceOptions)
	})
}

func apiVersionGetHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.APIVersionResourceType, v1.OperationGet, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewGetResource(opts, apiVersionResourceOptions)
	})
}

func apiVersionPutHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.APIVersionResourceType, v1.OperationPut, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultAsyncPut(opts, apiVersionResourceOptions)
	})
}

func apiVersionDeleteHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.APIVersionResourceType, v1.OperationDelete, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultAsyncDelete(opts, apiVersionResourceOptions)
	})
}

var locationResourceOptions = controller.ResourceOptions[datamodel.Location]{
	RequestConverter:         converter.LocationDataModelFromVersioned,
	ResponseConverter:        converter.LocationDataModelToVersioned,
	AsyncOperationRetryAfter: operationRetryAfter,
}

func locationListHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.LocationResourceType, v1.OperationList, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewListResources(opts, locationResourceOptions)
	})
}

func locationGetHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.LocationResourceType, v1.OperationGet, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewGetResource(opts, locationResourceOptions)
	})
}

func locationPutHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.LocationResourceType, v1.OperationPut, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultAsyncPut(opts, locationResourceOptions)
	})
}

func locationDeleteHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, datamodel.LocationResourceType, v1.OperationDelete, ctrlOptions, func(opts controller.Options) (controller.Controller, error) {
		return defaultoperation.NewDefaultAsyncDelete(opts, locationResourceOptions)
	})
}

func planeScopedProxyHandler(ctx context.Context, ctrlOptions controller.Options, transport http.RoundTripper, defaultDownstream string) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, OperationTypeUCPRadiusProxy, v1.OperationProxy, ctrlOptions, func(o controller.Options) (controller.Controller, error) {
		return radius_ctrl.NewProxyController(o, transport, defaultDownstream)
	})
}

func resourceGroupScopedProxyHandler(ctx context.Context, ctrlOptions controller.Options, transport http.RoundTripper, defaultDownstream string) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, OperationTypeUCPRadiusProxy, v1.OperationProxy, ctrlOptions, func(o controller.Options) (controller.Controller, error) {
		return radius_ctrl.NewProxyController(o, transport, defaultDownstream)
	})
}

func operationStatusGetHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	return server.CreateHandler(ctx, "System.Resources/operationstatuses", v1.OperationGet, ctrlOptions, defaultoperation.NewGetOperationStatus)
}

func operationResultGetHandler(ctx context.Context, ctrlOptions controller.Options) (http.HandlerFunc, error) {
	// NOTE: The resource type below is CORRECT. operation status and operation result use the same resource type in the database.
	return server.CreateHandler(ctx, "System.Resources/operationstatuses", v1.OperationGet, ctrlOptions, defaultoperation.NewGetOperationResult)
}
