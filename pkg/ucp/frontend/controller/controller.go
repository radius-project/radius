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

type HandlerOptions struct {
	server.HandlerOptions
	HandlerFactoryAWS ControllerAWSFunc
}

func RegisterHandler(ctx context.Context, opts HandlerOptions, ctrlOpts armrpc_controller.Options, awsOpts awsproxy.AWSOptions) error {
	storageClient, err := ctrlOpts.DataProvider.GetStorageClient(ctx, opts.ResourceType)
	if err != nil {
		return err
	}
	ctrlOpts.StorageClient = storageClient
	ctrlOpts.ResourceType = opts.ResourceType

	var ctrl armrpc_controller.Controller
	if opts.HandlerFactoryAWS != nil {
		ctrl, err = opts.HandlerFactoryAWS(ctrlOpts, awsOpts)
	} else {
		ctrl, err = opts.HandlerFactory(ctrlOpts)
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

	ot := v1.OperationType{Type: opts.Path, Method: opts.Method}
	if opts.Method != "" {
		opts.ParentRouter.Methods(opts.Method.HTTPMethod()).HandlerFunc(fn).Name(ot.String())
	} else {
		// Path is used to proxy plane request irrespective of the http method
		opts.ParentRouter.PathPrefix(opts.Path).HandlerFunc(fn).Name(ot.String())
	}
	return nil
}
