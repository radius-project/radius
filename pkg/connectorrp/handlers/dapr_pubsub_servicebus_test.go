// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

// import (
// 	"context"
// 	"log"
// 	"reflect"
// 	"testing"

// 	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
// 	"github.com/project-radius/radius/pkg/azure/armauth"
// 	"github.com/project-radius/radius/pkg/azure/clients"
// 	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
// 	"github.com/project-radius/radius/pkg/resourcekinds"
// 	"github.com/project-radius/radius/pkg/resourcemodel"
// 	"github.com/project-radius/radius/pkg/rp/outputresource"

// 	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
// 	"sigs.k8s.io/controller-runtime/pkg/client"
// )

// const (
// 	applicationName       = "test-application"
// 	resourceName          = "test-resource"
// 	serviceBusNamespaceID = "/subscriptions/fb8c1c54-2fe0-439b-aa40-a3e752986583/resourceGroups/test-resource-group/providers/Microsoft.ServiceBus/namespaces/test-namespace"
// 	kubernetesNamespace   = "radius-test"
// )

// func setupTest(tb testing.TB, initObjs ...client.Object) (func(tb testing.TB), ResourceHandler) {
// 	kubeFakeClient := radiustesting.NewKubeFakeClient()
// 	handler := NewDaprPubSubServiceBusHandler(&armauth.ArmConfig{}, kubeFakeClient.DynamicClient(initObjs...))

// 	return func(tb testing.TB) {
// 		log.Println("teardown test")
// 	}, handler
// }

// func Test_daprPubSubServiceBusHandler_Put(t *testing.T) {
// 	tests := []struct {
// 		name                       string
// 		daprComponent              *unstructured.Unstructured
// 		resource                   *outputresource.OutputResource
// 		wantOutputResourceIdentity *resourcemodel.ResourceIdentity
// 		wantProperties             map[string]string
// 		wantErr                    bool
// 	}{
// 		{
// 			name: "no-conflict",
// 			daprComponent: &unstructured.Unstructured{
// 				Object: map[string]interface{}{
// 					"apiVersion": "dapr.io/v1alpha1",
// 					"kind":       "Component",
// 					"metadata": map[string]interface{}{
// 						"namespace": "radius-test",
// 						"name":      "test-application-some-other-resource",
// 						"labels": map[string]interface{}{
// 							"radius.dev/resource-type": "applications.connector-daprPubSubBrokers",
// 						},
// 					},
// 				},
// 			},
// 			resource: &outputresource.OutputResource{
// 				ResourceType: resourcemodel.ResourceType{
// 					Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
// 					Provider: resourcemodel.ProviderAzure,
// 				},
// 				Resource: map[string]string{
// 					ServiceBusNamespaceIDKey: serviceBusNamespaceID,
// 					ApplicationName:          applicationName,
// 					ResourceName:             resourceName,
// 					KubernetesNamespaceKey:   kubernetesNamespace,
// 				},
// 			},
// 			wantOutputResourceIdentity: &resourcemodel.ResourceIdentity{
// 				ResourceType: &resourcemodel.ResourceType{
// 					Type:     resourcekinds.DaprPubSubTopicAzureServiceBus,
// 					Provider: resourcemodel.ProviderAzure,
// 				},
// 				Data: &resourcemodel.ARMIdentity{
// 					ID:         serviceBusNamespaceID,
// 					APIVersion: clients.GetAPIVersionFromUserAgent(servicebus.UserAgent()),
// 				},
// 			},
// 			wantProperties: map[string]string{
// 				ServiceBusNamespaceIDKey: serviceBusNamespaceID,
// 				ApplicationName:          applicationName,
// 				ResourceName:             resourceName,
// 				KubernetesNamespaceKey:   kubernetesNamespace,
// 			},
// 			wantErr: false,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			teardownTest, resourceHandler := setupTest(t, tt.daprComponent)
// 			defer teardownTest(t)

// 			gotOutputResourceIdentity, gotProperties, err := resourceHandler.Put(context.Background(), tt.resource)

// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("daprPubSubServiceBusHandler.Put() error = %v, wantErr %v", err, tt.wantErr)
// 				return
// 			}
// 			if !reflect.DeepEqual(gotOutputResourceIdentity, tt.wantOutputResourceIdentity) {
// 				t.Errorf("daprPubSubServiceBusHandler.Put() gotOutputResourceIdentity = %v, want %v", gotOutputResourceIdentity, tt.wantOutputResourceIdentity)
// 			}
// 			if !reflect.DeepEqual(gotProperties, tt.wantProperties) {
// 				t.Errorf("daprPubSubServiceBusHandler.Put() gotProperties = %v, want %v", gotProperties, tt.wantProperties)
// 			}
// 		})
// 	}
// }
