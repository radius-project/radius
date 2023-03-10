// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kube

import (
	"context"
	"errors"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	cdm "github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// FindNamespaceByEnvID finds the environment-scope Kubernetes namespace.
func FindNamespaceByEnvID(ctx context.Context, sp dataprovider.DataStorageProvider, envID string) (namespace string, err error) {
	id, err := resources.ParseResource(envID)
	if err != nil {
		return
	}

	if !strings.EqualFold(id.Type(), "Applications.Core/environments") {
		err = errors.New("invalid Applications.Core/environments resource id")
		return
	}

	env := &cdm.Environment{}
	client, err := sp.GetStorageClient(ctx, id.Type())
	if err != nil {
		return
	}

	res, err := client.Get(ctx, id.String())
	if err != nil {
		return
	}
	if err = res.As(env); err != nil {
		return
	}

	if env.Properties.Compute.Kind != rpv1.KubernetesComputeKind {
		err = errors.New("cannot get namespace because the current environment is not Kubernetes")
		return
	}

	namespace = id.Name()
	if env.Properties.Compute.KubernetesCompute.Namespace != "" {
		namespace = env.Properties.Compute.KubernetesCompute.Namespace
	}

	return
}

// FetchNameSpaceFromEnvironmentResource finds the environment-scope Kubernetes namespace from EnvironmentResource.
func FetchNameSpaceFromEnvironmentResource(environment *v20220315privatepreview.EnvironmentResource) (string, error) {
	kubernetes, ok := environment.Properties.Compute.(*v20220315privatepreview.KubernetesCompute)
	if !ok {
		return "", v1.ErrInvalidModelConversion
	}
	return *kubernetes.Namespace, nil
}

// FetchNameSpaceFromApplicationResource finds the application-scope Kubernetes namespace from ApplicationResource.
func FetchNameSpaceFromApplicationResource(application *v20220315privatepreview.ApplicationResource) (string, error) {
	kubernetes, ok := application.Properties.Status.Compute.(*v20220315privatepreview.KubernetesCompute)
	if !ok {
		return "", v1.ErrInvalidModelConversion
	}
	return *kubernetes.Namespace, nil
}
