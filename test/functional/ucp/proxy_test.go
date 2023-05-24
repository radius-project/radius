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

package ucp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/to"
	v20220901privatepreview "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8s "k8s.io/client-go/kubernetes"
)

const (
	TestRPServiceName = "testrp"
	TestRPPortNumber  = 5000
	RadiusNamespace   = "radius-system"
	Delay             = 10
)

func Test_ProxyOperations(t *testing.T) {
	test := NewUCPTest(t, "Test_ProxyOperations", func(t *testing.T, url string, roundTripper http.RoundTripper) {
		// Start a TestRP in the Radius namespace
		startTestRP(t, "")

		// Create a Non UCP-Native Plane with TestRP so that UCP will use the TestRP service address to forward requests to the RP
		nonNativePlaneID := "/planes/testnonnativetype/nonnativeplane"
		apiVersion := v20220901privatepreview.Version
		nonNativePlaneURL := fmt.Sprintf("%s%s?api-version=%s", url, nonNativePlaneID, apiVersion)

		nonNativePlane := v20220901privatepreview.PlaneResource{
			ID:       to.Ptr(nonNativePlaneID),
			Type:     to.Ptr("System.Planes/testnonnativetype"),
			Name:     to.Ptr("nonnativeplane"),
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &v20220901privatepreview.PlaneResourceProperties{
				Kind: to.Ptr(v20220901privatepreview.PlaneKindAWS),
				URL:  to.Ptr(fmt.Sprintf("http://%s.%s:%d", TestRPServiceName, RadiusNamespace, TestRPPortNumber)),
			},
		}

		createPlane(t, roundTripper, nonNativePlaneURL, nonNativePlane)
		t.Cleanup(func() {
			deletePlane(t, roundTripper, nonNativePlaneURL)
		})

		rgID := nonNativePlaneID + "/resourceGroups/test-rg"
		rgURL := fmt.Sprintf("%s%s", url, rgID)
		rgURLVersioned := fmt.Sprintf("%s?api-version=%s", rgURL, apiVersion)
		createResourceGroup(t, roundTripper, rgURLVersioned)
		t.Cleanup(func() {
			deleteResourceGroup(t, roundTripper, rgURLVersioned)
		})

		// Send a request which will be proxied to the TestRP
		issueGetRequest(t, roundTripper, url, rgURL, "", "", apiVersion)

		// Create UCP-native plane with TestRP so that UCP will use the TestRP service address to forward requests to the RP
		nativePlaneID := "/planes/testnativetype/testnativeplane"
		nativeplaneURL := fmt.Sprintf("%s%s?api-version=%s", url, nativePlaneID, apiVersion)
		nativePlane := v20220901privatepreview.PlaneResource{
			ID:       to.Ptr(nonNativePlaneID),
			Type:     to.Ptr("System.Planes/testnativetype"),
			Name:     to.Ptr("testnativeplane"),
			Location: to.Ptr(v1.LocationGlobal),
			Properties: &v20220901privatepreview.PlaneResourceProperties{
				Kind: to.Ptr(v20220901privatepreview.PlaneKindUCPNative),
				ResourceProviders: map[string]*string{
					"Applications.Test": to.Ptr(fmt.Sprintf("http://%s.%s:%d", TestRPServiceName, RadiusNamespace, TestRPPortNumber)),
				},
			},
		}
		createPlane(t, roundTripper, nativeplaneURL, nativePlane)
		t.Cleanup(func() {
			statusCode := deletePlane(t, roundTripper, nativeplaneURL)
			require.Equal(t, http.StatusOK, statusCode)

		})

		rgID = nativePlaneID + "/resourceGroups/test-rg"
		rgURL = fmt.Sprintf("%s%s", url, rgID)
		rgURLVersioned = fmt.Sprintf("%s?api-version=%s", rgURL, apiVersion)
		createResourceGroup(t, roundTripper, rgURLVersioned)
		t.Cleanup(func() {
			deleteResourceGroup(t, roundTripper, rgURLVersioned)
		})

		// Send a request which will be proxied to the TestRP and verify if the location header is translated by UCP
		issueGetRequest(t, roundTripper, url, rgURL, "Location", "location-header-value", apiVersion)
		issueGetRequest(t, roundTripper, url, rgURL, "Azure-Asyncoperation", "async-header-value", apiVersion)
	})
	test.Test(t)
}

func issueGetRequest(t *testing.T, roundTripper http.RoundTripper, url string, rgURL string, asyncHeaderName string, asyncHeaderValue string, apiVersion string) {
	var requestURL string
	if asyncHeaderName != "" {
		requestURL = fmt.Sprintf("%s/providers/Applications.Test/hello?%s=%s", rgURL, asyncHeaderName, asyncHeaderValue)
	} else {
		requestURL = fmt.Sprintf("%s/providers/Applications.Test/hello?api-version=%s", rgURL, apiVersion)
	}
	t.Logf("Fetching URL: %s", requestURL)

	getRequest, err := http.NewRequest(
		http.MethodGet,
		requestURL,
		nil,
	)
	require.NoError(t, err, "")

	result, err := roundTripper.RoundTrip(getRequest)
	require.NoError(t, err, "")
	require.Equal(t, http.StatusOK, result.StatusCode)

	if asyncHeaderName != "" {
		// Check if the async header is translated
		asyncHeader, foundAsyncHeader := result.Header[asyncHeaderName]
		require.True(t, foundAsyncHeader)
		expectedAsyncHeader := []string{url + "/" + asyncHeaderValue}
		require.Equal(t, expectedAsyncHeader, asyncHeader)
	}
}

func startTestRP(t *testing.T, configContext string) {
	ctx := context.Background()
	// Deploy a pod with the TestRP image to the k8s cluster where UCP is running
	config, err := kubernetes.NewCLIClientConfig(configContext)
	require.NoError(t, err)
	clientset, err := k8s.NewForConfig(config)
	require.NoError(t, err)

	testRPDeploymentName := "testrp"
	deploymentsClient := clientset.AppsV1().Deployments(RadiusNamespace)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testRPDeploymentName,
			Namespace: RadiusNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "testrp",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "testrp",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  testRPDeploymentName + "container",
							Image: getTestRPImage(),
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									Protocol:      corev1.ProtocolTCP,
									ContainerPort: TestRPPortNumber,
								},
							},
						},
					},
				},
			},
		},
	}

	t.Cleanup(func() {
		err = deploymentsClient.Delete(ctx, testRPDeploymentName, metav1.DeleteOptions{})
		require.NoError(t, err)
		t.Logf("Deployment %s deleted successfully", testRPDeploymentName)
	})

	// Create Deployment
	t.Log("Deploying Test RP...")
	result, err := deploymentsClient.Create(ctx, deployment, metav1.CreateOptions{})

	// Create a Service
	serviceClient := clientset.CoreV1().Services(RadiusNamespace)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TestRPServiceName,
			Namespace: RadiusNamespace,
			Labels: map[string]string{
				"app": "testrp",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       TestRPPortNumber,
					TargetPort: intstr.IntOrString{IntVal: TestRPPortNumber},
				},
			},
			Selector: map[string]string{
				"app": "testrp",
			},
			ClusterIP: "",
		},
	}

	t.Cleanup(func() {
		err = serviceClient.Delete(ctx, TestRPServiceName, metav1.DeleteOptions{})
		require.NoError(t, err)
		t.Logf("Service %s deleted successfully", TestRPServiceName)
	})

	// Create a service which will be used by UCP for forwarding requests
	_, err = serviceClient.Create(ctx, service, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Logf("Deployed Test RP %q.\n", result.GetObjectMeta().GetName())

	// Observed that the TestRP is not able to service requests coming in immediately after creation
	// Wait for some time for things to stabilize
	time.Sleep(time.Second * Delay)
}

func getTestRPImage() string {
	dockerReg := os.Getenv("DOCKER_REGISTRY")
	imageTag := os.Getenv("REL_VERSION")
	if dockerReg == "" {
		dockerReg = "radiusdev.azurecr.io"
	}
	if imageTag == "" {
		imageTag = "latest"
	}
	dockerImage := fmt.Sprintf("%s/testrp:%s", dockerReg, imageTag)
	return dockerImage
}
