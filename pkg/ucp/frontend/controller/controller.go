// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller/awsproxy"
)

type ControllerAWSFunc func(armrpc_controller.Options, awsproxy.AWSOptions) (armrpc_controller.Controller, error)

type AWSHandlerOptions struct {
	HandlerFactoryAWS ControllerAWSFunc
}

type HandlerOptions struct {
	Options    server.HandlerOptions
	AWSOptions AWSHandlerOptions
}

func RegisterHandler(ctx context.Context, opts HandlerOptions, ctrlOpts armrpc_controller.Options, awsOpts awsproxy.AWSOptions) error {
	storageClient, err := ctrlOpts.DataProvider.GetStorageClient(ctx, opts.Options.ResourceType)
	if err != nil {
		return err
	}
	ctrlOpts.StorageClient = storageClient
	ctrlOpts.ResourceType = opts.Options.ResourceType

	var ctrl armrpc_controller.Controller
	if opts.AWSOptions.HandlerFactoryAWS != nil {
		ctrl, err = opts.AWSOptions.HandlerFactoryAWS(ctrlOpts, awsOpts)
	} else {
		ctrl, err = opts.Options.HandlerFactory(ctrlOpts)
	}
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

	ot := v1.OperationType{Type: opts.Options.Path, Method: opts.Options.Method}
	if opts.Options.Method != "" {
		opts.Options.ParentRouter.Methods(opts.Options.Method.HTTPMethod()).HandlerFunc(fn).Name(ot.String())
	} else {
		// Path is used to proxy plane request irrespective of the http method
		opts.Options.ParentRouter.PathPrefix(opts.Options.Path).HandlerFunc(fn).Name(ot.String())
	}
	return nil
}
