// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kube

import (
	"context"
	"errors"
	"strings"

	cdm "github.com/project-radius/radius/pkg/corerp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// FindNamespaceByEnvID finds the environment-scope Kuberentes namespace.
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
