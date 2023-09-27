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

package applications

import (
	"context"
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/frontend/controller/util"
	"github.com/radius-project/radius/pkg/kubernetes"
	rp_kube "github.com/radius-project/radius/pkg/rp/kube"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	envNamespaceQuery = "properties.compute.kubernetes.namespace"
	appNamespaceQuery = "properties.status.compute.kubernetes.namespace"
)

// Radius uses Kubernetes namespace by following rules:
// +-----------------+--------------------+-------------------------------+-------------------------------+
// | namespace       | namespace override | env-scoped resource namespace | app-scoped resource namespace |
// | in Environments | in Applications    |                               |                               |
// +-----------------+--------------------+-------------------------------+-------------------------------+
// | envNS           | UNDEFINED          | envNS                         | envNS-{appName}               |
// | envNS           | appNS              | envNS                         | appNS                         |
// +-----------------+--------------------+-------------------------------+-------------------------------+

// CreateAppScopedNamespace checks if a namespace already exists for the application and creates one if it doesn't,
// returning an error if a conflict is found.
func CreateAppScopedNamespace(ctx context.Context, newResource, oldResource *datamodel.Application, opt *controller.Options) (rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	kubeNamespace := ""
	ext := datamodel.FindExtension(newResource.Properties.Extensions, datamodel.KubernetesNamespaceExtension)
	if ext != nil {
		// Override environment namespace.
		kubeNamespace = ext.KubernetesNamespace.Namespace
	} else {
		// Construct namespace using the namespace specified by environment resource.
		envNamespace, err := rp_kube.FindNamespaceByEnvID(ctx, opt.DataProvider, newResource.Properties.Environment)
		if err != nil {
			return rest.NewBadRequestResponse(fmt.Sprintf("Environment %s could not be constructed: %s",
				newResource.Properties.Environment, err.Error())), nil
		}

		namespace := fmt.Sprintf("%s-%s", envNamespace, serviceCtx.ResourceID.Name())
		if !kubernetes.IsValidObjectName(namespace) {
			return rest.NewBadRequestResponse(fmt.Sprintf("Application namespace '%s' could not be created: the combination of application and environment names is too long.",
				namespace)), nil
		}

		kubeNamespace = kubernetes.NormalizeResourceName(namespace)
	}

	// Check if another environment resource is using namespace
	envID, err := resources.ParseResource(newResource.Properties.Environment)
	if err != nil {
		return rest.NewBadRequestResponse(fmt.Sprintf("Environment %s for application %s could not be found", envID.Name(), serviceCtx.ResourceID.Name())), nil
	}

	result, err := util.FindResources(ctx, envID.RootScope(), envID.Type(), envNamespaceQuery, kubeNamespace, opt.StorageClient)
	if err != nil {
		return nil, err
	}
	if len(result.Items) > 0 {
		return rest.NewConflictResponse(fmt.Sprintf("Environment %s with the same namespace (%s) already exists", envID.Name(), kubeNamespace)), nil
	}

	// Check if another application resource is using namespace
	result, err = util.FindResources(ctx, serviceCtx.ResourceID.RootScope(), serviceCtx.ResourceID.Type(), appNamespaceQuery, kubeNamespace, opt.StorageClient)
	if err != nil {
		return nil, err
	}
	if len(result.Items) > 0 {
		app := &datamodel.Application{}
		if err := result.Items[0].As(app); err != nil {
			return nil, err
		}

		// If a different resource has the same namespace, return a conflict
		// Otherwise, continue and update the resource
		if oldResource == nil || app.ID != oldResource.ID {
			return rest.NewConflictResponse(fmt.Sprintf("Application %s with the same namespace (%s) already exists", app.ID, kubeNamespace)), nil
		}
	}

	if !kubernetes.IsValidObjectName(kubeNamespace) {
		return rest.NewBadRequestResponse(fmt.Sprintf("'%s' is the invalid namespace. This must be at most 63 alphanumeric characters or '-'. Please specify a valid namespace using 'kubernetesNamespace' extension in '$.properties.extensions[*]'.", kubeNamespace)), nil
	}

	if oldResource != nil {
		c := oldResource.Properties.Status.Compute
		if c != nil && c.Kind == rpv1.KubernetesComputeKind && c.KubernetesCompute.Namespace != kubeNamespace {
			return rest.NewBadRequestResponse(fmt.Sprintf("Updating an application's Kubernetes namespace from '%s' to '%s' requires the application to be deleted and redeployed. Please delete your application and try again.", c.KubernetesCompute.Namespace, kubeNamespace)), nil
		}
	}

	// Populate kubernetes namespace to internal metadata property for query indexing.
	newResource.Properties.Status.Compute = &rpv1.EnvironmentCompute{
		Kind:              rpv1.KubernetesComputeKind,
		KubernetesCompute: rpv1.KubernetesComputeProperties{Namespace: kubeNamespace},
	}

	// TODO: Move it to backend controller - https://github.com/radius-project/radius/issues/4742
	err = opt.KubeClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: kubeNamespace}})
	if apierrors.IsAlreadyExists(err) {
		logger.Info("Using existing namespace", "namespace", kubeNamespace)
	} else if err != nil {
		return nil, err
	} else {
		logger.Info("Created the namespace", "namespace", kubeNamespace)
	}

	return nil, nil
}
