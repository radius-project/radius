// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package client

import (
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	csidriver "sigs.k8s.io/secrets-store-csi-driver/apis/v1alpha1"
)

// CreateKubeClient creates new kubernetes clients.
func CreateKubeClient(config *rest.Config) (runtimeclient.Client, error) {
	scheme := runtime.NewScheme()

	// TODO: add required resource scheme.
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(csidriver.AddToScheme(scheme))
	utilruntime.Must(contourv1.AddToScheme(scheme))

	return runtimeclient.New(config, runtimeclient.Options{Scheme: scheme})
}
