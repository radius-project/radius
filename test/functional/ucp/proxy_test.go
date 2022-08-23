// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/ucp/rest"
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
		nonNativePlaneID := "/planes/testNonNativeType/nonNativePlane"
		nonNativePlaneURL := fmt.Sprintf("%s%s", url, nonNativePlaneID)
		nonNativePlane := rest.Plane{
			ID:   nonNativePlaneID,
			Type: "System.Planes/testNonNativeType",
			Name: "nonNativePlane",
			Properties: rest.PlaneProperties{
				Kind: "testNonNativeType",
				URL:  fmt.Sprintf("http://%s.%s:%d", TestRPServiceName, RadiusNamespace, TestRPPortNumber),
			},
		}
		createPlane(t, roundTripper, nonNativePlaneURL, nonNativePlane, false)
		t.Cleanup(func() {
			deletePlane(t, roundTripper, nonNativePlaneURL)
		})

		rgID := nonNativePlaneID + "/resourceGroups/test-rg"
		rgURL := fmt.Sprintf("%s%s", url, rgID)
		createResourceGroup(t, roundTripper, rgURL, false)
		t.Cleanup(func() {
			deleteResourceGroup(t, roundTripper, rgURL)
		})

		// Send a request which will be proxied to the TestRP
		issueGetRequest(t, roundTripper, url, rgURL, "", "")

		// Create UCP-native plane with TestRP so that UCP will use the TestRP service address to forward requests to the RP
		nativePlaneID := "/planes/testNativeType/testNativePlane"
		nativeplaneURL := fmt.Sprintf("%s%s", url, nativePlaneID)
		nativePlane := rest.Plane{
			ID:   nonNativePlaneID,
			Type: "System.Planes/testNativeType",
			Name: "testNativePlane",
			Properties: rest.PlaneProperties{
				Kind: rest.PlaneKindUCPNative,
				ResourceProviders: map[string]string{
					"Applications.Test": fmt.Sprintf("http://%s.%s:%d", TestRPServiceName, RadiusNamespace, TestRPPortNumber),
				},
			},
		}
		createPlane(t, roundTripper, nativeplaneURL, nativePlane, false)
		t.Cleanup(func() {
			deletePlane(t, roundTripper, nativeplaneURL)
		})

		rgID = nativePlaneID + "/resourceGroups/test-rg"
		rgURL = fmt.Sprintf("%s%s", url, rgID)
		createResourceGroup(t, roundTripper, rgURL, false)
		t.Cleanup(func() {
			deleteResourceGroup(t, roundTripper, rgURL)
		})

		// Send a request which will be proxied to the TestRP and verify if the location header is translated by UCP
		issueGetRequest(t, roundTripper, url, rgURL, "Location", "location-header-value")
		issueGetRequest(t, roundTripper, url, rgURL, "Azure-Asyncoperation", "async-header-value")
	})
	test.Test(t)
}

func issueGetRequest(t *testing.T, roundTripper http.RoundTripper, url string, rgURL string, asyncHeaderName string, asyncHeaderValue string) {
	var requestURL string
	if asyncHeaderName != "" {
		requestURL = fmt.Sprintf("%s/providers/Applications.Test/hello?%s=%s", rgURL, asyncHeaderName, asyncHeaderValue)
	} else {
		requestURL = fmt.Sprintf("%s/providers/Applications.Test/hello", rgURL)
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
	config, err := kubernetes.GetConfig(configContext)
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
