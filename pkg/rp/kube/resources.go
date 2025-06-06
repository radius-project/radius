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

package kube

import (
	"context"
	"errors"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	cdm "github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var (
	ErrNonKubernetesEnvironment = errors.New("cannot get namespace because the current environment is not Kubernetes")
)

// FindNamespaceByEnvID finds the environment-scope Kubernetes namespace. If the environment ID is invalid or the environment is not a Kubernetes
// environment, an error is returned.
func FindNamespaceByEnvID(ctx context.Context, databaseClient database.Client, envID string) (namespace string, err error) {
	id, err := resources.ParseResource(envID)
	if err != nil {
		return
	}

	if !strings.EqualFold(id.Type(), "Applications.Core/environments") {
		err = errors.New("invalid Applications.Core/environments resource id")
		return
	}

	env := &cdm.Environment{}
	res, err := databaseClient.Get(ctx, id.String())
	if err != nil {
		return
	}
	if err = res.As(env); err != nil {
		return
	}

	if env.Properties.Compute.Kind != rpv1.KubernetesComputeKind {
		err = ErrNonKubernetesEnvironment
		return
	}

	namespace = id.Name()
	if env.Properties.Compute.KubernetesCompute.Namespace != "" {
		namespace = env.Properties.Compute.KubernetesCompute.Namespace
	}

	return
}

// FetchNamespaceFromEnvironmentResource finds the environment-scope Kubernetes namespace from EnvironmentResource.
// If no namespace is found, an error is returned.
func FetchNamespaceFromEnvironmentResource(environment *v20231001preview.EnvironmentResource) (string, error) {
	if environment.Properties.Compute != nil {
		kubernetes, ok := environment.Properties.Compute.(*v20231001preview.KubernetesCompute)
		if !ok {
			return "", v1.ErrInvalidModelConversion
		}
		return *kubernetes.Namespace, nil
	}
	return "", errors.New("unable to fetch namespace information")

}

// FetchNamespaceFromApplicationResource finds the application-scope Kubernetes namespace from ApplicationResource.
// If no namespace is found, an error is returned.
func FetchNamespaceFromApplicationResource(application *v20231001preview.ApplicationResource) (string, error) {
	if application.Properties.Status != nil && application.Properties.Status.Compute != nil {
		kubernetes, ok := application.Properties.Status.Compute.(*v20231001preview.KubernetesCompute)
		if !ok {
			return "", v1.ErrInvalidModelConversion
		}
		return *kubernetes.Namespace, nil
	}
	return "", errors.New("unable to fetch namespace information")
}
