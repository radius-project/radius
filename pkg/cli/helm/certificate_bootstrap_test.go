/*
Copyright 2026 The Radius Authors.

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

package helm

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func bootstrapStoreFixture(t *testing.T) *rotationStore {
	t.Helper()
	kube := kubefake.NewSimpleClientset()
	client := fakeCertManagerDynamicClient(nil)
	version := 0
	metadata := func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetVerb() != "create" && action.GetVerb() != "update" {
			return false, nil, nil
		}
		object := action.(interface{ GetObject() runtime.Object }).GetObject()
		accessor, err := meta.Accessor(object)
		require.NoError(t, err)
		if accessor.GetUID() == "" {
			accessor.SetUID(types.UID(uuid.NewString()))
		}
		version++
		accessor.SetResourceVersion(fmt.Sprint(version))
		return false, nil, nil
	}
	kube.PrependReactor("*", "*", metadata)
	client.PrependReactor("*", "*", metadata)
	return &rotationStore{kube: kube, dynamic: client, namespace: RadiusSystemNamespace, release: radiusReleaseName, now: time.Now}
}
