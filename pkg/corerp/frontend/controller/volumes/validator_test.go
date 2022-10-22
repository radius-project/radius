// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package volumes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	corerptesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/ucp/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	scheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	secretsstorev1 "sigs.k8s.io/secrets-store-csi-driver/apis/v1"
)

var (
	initObjects = []client.Object{
		&secretsstorev1.SecretProviderClass{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "simple_provider",
				Namespace: "default",
			},
			Spec: secretsstorev1.SecretProviderClassSpec{
				Provider:   "simple_provider",
				Parameters: map[string]string{"parameter1": "value1"},
			},
		},
	}
	resourceID = "/subscriptions/test-subscription-id/resourceGroups/test-resource-group/providers/applications.core/volumes/test-volume"
)

func getKubeClientWithScheme(initObjs ...client.Object) client.WithWatch {
	s := scheme.Scheme
	s.AddKnownTypes(schema.GroupVersion{Group: secretsstorev1.GroupVersion.Group, Version: secretsstorev1.GroupVersion.Version},
		&secretsstorev1.SecretProviderClass{},
		&secretsstorev1.SecretProviderClassList{},
		&secretsstorev1.SecretProviderClassPodStatus{},
	)

	return fakeclient.NewClientBuilder().
		WithScheme(s).
		WithObjects(initObjs...).
		Build()
}

func getResourceID(id string) resources.ID {
	resourceID, err := resources.ParseResource(id)
	if err != nil {
		panic(err)
	}

	return resourceID
}

func TestValidateRequest(t *testing.T) {
	type args struct {
		ctx         context.Context
		newResource *datamodel.VolumeResource
		oldResource *datamodel.VolumeResource
		options     *controller.Options
	}
	tests := []struct {
		name    string
		args    args
		want    rest.Response
		wantErr error
	}{
		{
			name: "unsuppoted-operation",
			args: args{
				ctx: v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						ResourceID:    getResourceID(resourceID),
						OperationType: http.MethodDelete,
					}),
				newResource: &datamodel.VolumeResource{
					Properties: datamodel.VolumeResourceProperties{
						Kind: datamodel.AzureKeyVaultVolume,
						AzureKeyVault: &datamodel.AzureKeyVaultVolumeProperties{
							Identity: datamodel.AzureIdentity{
								Kind:     datamodel.AzureIdentitySystemAssigned,
								ClientID: "123",
							},
						},
					},
				},
				oldResource: &datamodel.VolumeResource{},
				options: &controller.Options{
					KubeClient: corerptesting.NewKubeFakeClient().DynamicClient(),
				},
			},
			want:    rest.NewMethodNotAllowedResponse(resourceID, "only PUT and PATCH are supported for the validation of this resource."),
			wantErr: nil,
		},
		{
			name: "invalid-kind",
			args: args{
				ctx: v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						ResourceID:    getResourceID(resourceID),
						OperationType: http.MethodPut,
					}),
				newResource: &datamodel.VolumeResource{
					Properties: datamodel.VolumeResourceProperties{
						Kind: "unsupported-kind",
						AzureKeyVault: &datamodel.AzureKeyVaultVolumeProperties{
							Identity: datamodel.AzureIdentity{
								Kind:     datamodel.AzureIdentitySystemAssigned,
								ClientID: "123",
							},
						},
					},
				},
				oldResource: &datamodel.VolumeResource{},
				options: &controller.Options{
					KubeClient: corerptesting.NewKubeFakeClient().DynamicClient(),
				},
			},
			want:    rest.NewBadRequestResponse(fmt.Sprintf("invalid resource kind: %s", "unsupported-kind")),
			wantErr: nil,
		},
		{
			name: "system-assigned-client-id-not-empty",
			args: args{
				ctx: v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						ResourceID:    getResourceID(resourceID),
						OperationType: http.MethodPut,
					}),
				newResource: &datamodel.VolumeResource{
					Properties: datamodel.VolumeResourceProperties{
						Kind: datamodel.AzureKeyVaultVolume,
						AzureKeyVault: &datamodel.AzureKeyVaultVolumeProperties{
							Identity: datamodel.AzureIdentity{
								Kind:     datamodel.AzureIdentitySystemAssigned,
								ClientID: "123",
							},
						},
					},
				},
				oldResource: &datamodel.VolumeResource{},
				options: &controller.Options{
					KubeClient: corerptesting.NewKubeFakeClient().DynamicClient(),
				},
			},
			want:    rest.NewBadRequestResponse("clientID is not allowed when using system assigned identity"),
			wantErr: nil,
		},
		{
			name: "workload-client-id-empty",
			args: args{
				ctx: v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						ResourceID:    getResourceID(resourceID),
						OperationType: http.MethodPut,
					}),
				newResource: &datamodel.VolumeResource{
					Properties: datamodel.VolumeResourceProperties{
						Kind: datamodel.AzureKeyVaultVolume,
						AzureKeyVault: &datamodel.AzureKeyVaultVolumeProperties{
							Identity: datamodel.AzureIdentity{
								Kind:     datamodel.AzureIdentityWorkload,
								ClientID: "",
							},
						},
					},
				},
				oldResource: &datamodel.VolumeResource{},
				options: &controller.Options{
					KubeClient: corerptesting.NewKubeFakeClient().DynamicClient(),
				},
			},
			want:    rest.NewBadRequestResponse("clientID can not be empty when using workload identity"),
			wantErr: nil,
		},
		{
			name: "csi-driver-not-installed",
			args: args{
				ctx: v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						ResourceID:    getResourceID(resourceID),
						OperationType: http.MethodPut,
					}),
				newResource: &datamodel.VolumeResource{
					Properties: datamodel.VolumeResourceProperties{
						Kind: datamodel.AzureKeyVaultVolume,
						AzureKeyVault: &datamodel.AzureKeyVaultVolumeProperties{
							Identity: datamodel.AzureIdentity{
								Kind:     datamodel.AzureIdentitySystemAssigned,
								ClientID: "",
							},
						},
					},
				},
				oldResource: &datamodel.VolumeResource{},
				options: &controller.Options{
					KubeClient: getKubeClientWithScheme(),
				},
			},
			want:    rest.NewPreconditionFailedResponse(resourceID, errors.New("csi driver is not installed").Error()),
			wantErr: nil,
		},
		{
			name: "csi-driver-installed",
			args: args{
				ctx: v1.WithARMRequestContext(
					context.Background(), &v1.ARMRequestContext{
						ResourceID:    getResourceID(resourceID),
						OperationType: http.MethodPut,
					}),
				newResource: &datamodel.VolumeResource{
					Properties: datamodel.VolumeResourceProperties{
						Kind: datamodel.AzureKeyVaultVolume,
						AzureKeyVault: &datamodel.AzureKeyVaultVolumeProperties{
							Identity: datamodel.AzureIdentity{
								Kind:     datamodel.AzureIdentitySystemAssigned,
								ClientID: "",
							},
						},
					},
				},
				oldResource: &datamodel.VolumeResource{},
				options: &controller.Options{
					KubeClient: getKubeClientWithScheme(initObjects...),
				},
			},
			want:    nil,
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := ValidateRequest(tt.args.ctx, tt.args.newResource, tt.args.oldResource, tt.args.options); !reflect.DeepEqual(got, tt.want) &&
				(tt.wantErr != nil && tt.wantErr.Error() != err.Error()) {
				t.Errorf("ValidateRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}
