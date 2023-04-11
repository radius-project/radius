// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testutil

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// NewFakeKubeClient create new fake kube dynamic client.
func NewFakeKubeClient(scheme *runtime.Scheme, initObjs ...client.Object) client.WithWatch {
	builder := fake.NewClientBuilder()
	if scheme != nil {
		builder = builder.WithScheme(scheme)
	}
	return builder.WithObjects(initObjs...).Build()
}

// PrependPatchReactor prepends patch reactor to fake client. This is workaround because clientset
// fake doesn't support patch verb. https://github.com/kubernetes/client-go/issues/1184
func PrependPatchReactor(f *k8sfake.Clientset, resource string, objFunc func(clienttesting.PatchAction) runtime.Object) {
	f.PrependReactor(
		"patch",
		resource,
		func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
			pa := action.(clienttesting.PatchAction)
			if pa.GetPatchType() == types.ApplyPatchType {
				rfunc := clienttesting.ObjectReaction(f.Tracker())
				_, obj, err := rfunc(
					clienttesting.NewGetAction(pa.GetResource(), pa.GetNamespace(), pa.GetName()),
				)
				if apierrors.IsNotFound(err) || obj == nil {
					_, _, _ = rfunc(
						clienttesting.NewCreateAction(
							pa.GetResource(),
							pa.GetNamespace(),
							objFunc(pa),
						),
					)
				}
				return rfunc(clienttesting.NewPatchAction(
					pa.GetResource(),
					pa.GetNamespace(),
					pa.GetName(),
					types.StrategicMergePatchType,
					pa.GetPatch()))
			}
			return false, nil, nil
		},
	)
}
