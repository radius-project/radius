// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/kubernetes"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_ConvertK8sApplicationToARM(t *testing.T) {
	original := radiusv1alpha3.Application{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "radius.dev/v1alpha1",
			Kind:       "Application",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "frontend-backend",
			Namespace: "default",
			Annotations: map[string]string{
				kubernetes.LabelRadiusApplication: "frontend-backend",
			},
		},
		Spec: radiusv1alpha3.ApplicationSpec{},
	}

	expected := &radclient.ApplicationResource{
		TrackedResource: radclient.TrackedResource{
			Resource: radclient.Resource{
				Name: to.StringPtr("frontend-backend"),
			},
		},
		Properties: &radclient.ApplicationProperties{},
	}

	actual, err := ConvertK8sApplicationToARM(original)
	require.NoError(t, err, "failed to convert application")

	require.Equal(t, expected, actual)
}
