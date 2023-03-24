// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	ucp_aws "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/validator"
)

// Options represents controller options.
type Options struct {
	// BasePath is the base path of the controller such as /apis/api.ucp.dev/v1alpha3.
	BasePath string

	// Address is the address where the controller is running.
	Address string

	// SecretClient is the client to fetch secrets.
	SecretClient secret.Client

	// AWSOptions is the set of options used by AWS controllers.
	AWSOptions AWSOptions

	// BaseControllerOptions is the set of options used by all controllers.
	armrpc_controller.Options
}

type AWSOptions struct {
	// AWSCloudControlClient is the AWS Cloud Control client.
	AWSCloudControlClient ucp_aws.AWSCloudControlClient

	// AWSCloudFormationClient is the AWS Cloud Formation client.
	AWSCloudFormationClient ucp_aws.AWSCloudFormationClient
}

type ControllerFunc func(Options) (armrpc_controller.Controller, error)

type HandlerOptions struct {
	ParentRouter   *mux.Router
	ResourceType   string
	Path           string
	Method         v1.OperationMethod
	HandlerFactory ControllerFunc
}

func RegisterHandler(ctx context.Context, opts HandlerOptions, ctrlOpts Options) error {
	storageClient, err := ctrlOpts.DataProvider.GetStorageClient(ctx, opts.ResourceType)
	if err != nil {
		return err
	}
	ctrlOpts.StorageClient = storageClient
	ctrlOpts.ResourceType = opts.ResourceType

	ctrl, err := opts.HandlerFactory(ctrlOpts)
	if err != nil {
		return err
	}

	fn := func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		response, err := ctrl.Run(ctx, w, req)
		if err != nil {
			server.HandleError(ctx, w, req, err)
			return
		}
		if response != nil {
			err = response.Apply(ctx, w, req)
			if err != nil {
				server.HandleError(ctx, w, req, err)
				return
			}
		}
	}

	ot := v1.OperationType{Type: opts.Path, Method: opts.Method}
	if opts.Method != "" {
		opts.ParentRouter.Methods(opts.Method.HTTPMethod()).HandlerFunc(fn).Name(ot.String())
	} else {
		// Path is used to proxy plane request irrespective of the http method
		opts.ParentRouter.PathPrefix(opts.Path).HandlerFunc(fn).Name(ot.String())
	}
	return nil
}

func ConfigureDefaultHandlers(router *mux.Router, opts armrpc_controller.Options) {
	router.NotFoundHandler = validator.APINotFoundHandler()
	router.MethodNotAllowedHandler = validator.APIMethodNotAllowedHandler()
}
