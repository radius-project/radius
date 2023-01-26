// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testutil

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// NewFakeKubeClient create new fake kube dynamic client.
func NewFakeKubeClient(scheme *runtime.Scheme, initObjs ...client.Object) client.WithWatch {
	builder := fake.NewClientBuilder()
	if scheme != nil {
		builder = builder.WithScheme(scheme)
	}
	return builder.WithObjects(initObjs...).Build()
}
