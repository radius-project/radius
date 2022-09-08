// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"errors"
	"net/http"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	sm "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Operation is the base operation controller.
type Operation[P interface {
	*T
	conv.DataModelInterface
}, T any] struct {
	options Options

	// RequestConverter is the converter to convert from the versioned API resource to datamodel resource.
	RequestConverter conv.RequestConverter[T]
	// ResponseConverter is the converter to convert from datamodel resource to versioned API for response.
	ResponseConverter conv.ResponseConverter[T]
}

// NewOperation creates BaseController instance.
func NewOperation[P interface {
	*T
	conv.DataModelInterface
}, T any](options Options, reqconv conv.RequestConverter[T], respconv conv.ResponseConverter[T]) Operation[P, T] {
	return Operation[P, T]{options, reqconv, respconv}
}

// StorageClient gets storage client for this controller.
func (b *Operation[P, T]) StorageClient() store.StorageClient {
	return b.options.StorageClient
}

// DataProvider gets data storage provider for this controller.
func (b *Operation[P, T]) DataProvider() dataprovider.DataStorageProvider {
	return b.options.DataProvider
}

// SecretClient gets secret client for this controller.
func (b *Operation[P, T]) SecretClient() rp.SecretValueClient {
	return b.options.SecretClient
}

// KubeClient gets Kubernetes client for this controller.
func (b *Operation[P, T]) KubeClient() runtimeclient.Client {
	return b.options.KubeClient
}

// ResourceType gets the resource type for this controller.
func (b *Operation[P, T]) ResourceType() string {
	return b.options.ResourceType
}

// DeploymentProcessor gets the deployment processor for this controller.
func (b *Operation[P, T]) DeploymentProcessor() deployment.DeploymentProcessor {
	return b.options.GetDeploymentProcessor()
}

// DeploymentProcessor gets the deployment processor for this controller.
func (b *Operation[P, T]) StatusManager() sm.StatusManager {
	return b.options.StatusManager
}

// GetResourceFromRequest extracts and deserializes from HTTP request body to datamodel.
func (c *Operation[P, T]) GetResourceFromRequest(ctx context.Context, req *http.Request) (*T, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	content, err := ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := c.RequestConverter(content, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}
	return dm, nil
}

// GetResource is the helper to get the resource via storage client.
func (c *Operation[P, T]) GetResourceFromStore(ctx context.Context, id resources.ID) (out *T, etag string, isNotFound bool, err error) {
	etag = ""
	out = new(T)
	isNotFound = false
	var res *store.Object
	if res, err = c.StorageClient().Get(ctx, id.String()); err == nil {
		if err = res.As(out); err == nil {
			etag = res.ETag
			return
		}
	}

	out = nil
	if errors.Is(&store.ErrNotFound{}, err) {
		isNotFound = true
		err = nil
	}
	return
}

// ValidateResource runs the common validation logic for incoming request.
func (c *Operation[P, T]) ValidateResource(ctx context.Context, req *http.Request, newResource *T, oldResource *T, etag string, isNew bool) error {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	if req.Method == http.MethodPatch && isNew {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID)
	}

	if err := ValidateETag(*serviceCtx, etag); err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error())
	}

	return nil
}

// ValidateLinkedResource checks if application and environment id in new resource are matched with the old resource.
func (c *Operation[P, T]) ValidateLinkedResource(resourceID resources.ID, isNew bool, newProp *v1.BasicResourceProperties, oldProp *v1.BasicResourceProperties) error {
	if !isNew && !oldProp.EqualLinkedResource(newProp) {
		return rest.NewLinkedResourceUpdateErrorResponse(resourceID, oldProp, newProp)
	}
	return nil
}

// ConstructSyncResponse constructs synchronous API response.
func (c *Operation[P, T]) ConstructSyncResponse(ctx context.Context, method, etag string, resource *T) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	versioned, err := c.ResponseConverter(resource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"ETag": etag}
	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// ConstructAsyncResponse constructs asynchronous API response.
func (c *Operation[P, T]) ConstructAsyncResponse(ctx context.Context, method, etag string, resource *T) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	respCode := http.StatusAccepted
	if method == http.MethodPut {
		respCode = http.StatusCreated
	}

	return rest.NewAsyncOperationResponse(resource, serviceCtx.Location, respCode,
		serviceCtx.ResourceID, serviceCtx.OperationID, serviceCtx.APIVersion), nil
}

// SaveResource is the helper to save the resource via storage client.
func (c *Operation[P, T]) SaveResource(ctx context.Context, id string, in *T, etag string) (*store.Object, error) {
	nr := &store.Object{
		Metadata: store.Metadata{
			ID: id,
		},
		Data: in,
	}
	err := c.StorageClient().Save(ctx, nr, store.WithETag(etag))
	if err != nil {
		return nil, err
	}
	return nr, nil
}
