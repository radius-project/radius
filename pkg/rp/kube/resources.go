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
	ctrl_app "github.com/project-radius/radius/pkg/corerp/frontend/controller/applications"
	ctrl_env "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

// Radius uses Kuberentes namespace by following rules:
// +-----------------+--------------------+------------------------------+------------------------------+
// | namespace       | namespace override | env-scope resource namespace | app-scope resource namespace |
// | in Environments | in Applications    |                              |                              |
// +-----------------+--------------------+------------------------------+------------------------------+
// | UNDEFINED       | UNDEFINED          | {envName}                    | {envName}-{appName}          |
// | envNS           | UNDEFINED          | envNS                        | envNS-{appName}              |
// | envNS           | appNS              | envNS                        | appNS                        |
// +-----------------+--------------------+------------------------------+------------------------------+

// FindNamespaceByEnvID finds the environment-scope Kuberentes namespace.
func FindNamespaceByEnvID(ctx context.Context, sp dataprovider.DataStorageProvider, envID string) (namespace string, err error) {
	id, err := resources.ParseResource(envID)
	if err != nil {
		return
	}

	if !strings.EqualFold(id.Type(), ctrl_env.ResourceTypeName) {
		err = errors.New("invalid Applications.Core/Environments resource id")
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

	if env.Properties.Compute.Kind != cdm.KubernetesComputeKind {
		err = errors.New("cannot get namespace because the current environment is not Kubernetes")
		return
	}

	namespace = id.Name()
	if env.Properties.Compute.KubernetesCompute.Namespace != "" {
		namespace = env.Properties.Compute.KubernetesCompute.Namespace
	}

	return
}

// FindNamespaceByAppID finds the application-scope Kuberentes namespace.
func FindNamespaceByAppID(ctx context.Context, sp dataprovider.DataStorageProvider, appID string) (namespace string, err error) {
	id, err := resources.ParseResource(appID)
	if err != nil {
		return
	}

	if !strings.EqualFold(id.Type(), ctrl_app.ResourceTypeName) {
		err = errors.New("invalid Applications.Core/Applications resource id")
		return
	}

	suffix := ""

	app := &cdm.Application{}
	client, err := sp.GetStorageClient(ctx, id.Type())
	if err != nil {
		return
	}

	res, err := client.Get(ctx, id.String())
	if err != nil {
		return
	}
	if err = res.As(app); err != nil {
		return
	}
	ext := app.Properties.FindExtension(cdm.KubernetesNamespaceOverride)
	if ext == nil {
		suffix = id.Name()
	} else if ext.KubernetesNamespaceOverride != nil {
		namespace = ext.KubernetesNamespaceOverride.Namespace
		return
	}

	namespace, err = FindNamespaceByEnvID(ctx, sp, app.Properties.Environment)
	namespace += "-" + suffix
	return
}
