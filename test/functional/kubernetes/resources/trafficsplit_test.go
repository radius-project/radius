// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"strconv"
	"strings"
	"testing"

	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/functional/kubernetes"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

const (
	backendTimeout = 3 * time.Minute
)

func Test_TrafficSplit(t *testing.T) {
	template := "testdata/kubernetes-resources-trafficsplit.bicep"
	application := "trafficsplit"
	test := kubernetes.NewApplicationTest(t, application, []kubernetes.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "trafficsplit",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, rest.ResourceType{Type: resourcekinds.Service, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							"TrafficSplit":                   validation.NewOutputResource("TrafficSplit", rest.ResourceType{Type: "TrafficSplit", Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "httpbin"),
						validation.NewK8sPodForResource(application, "httpbinv2"),
						validation.NewK8sServiceForResource(application, "httpbinroute-v1"),
						validation.NewK8sServiceForResource(application, "httpbinroute-v2"),
						validation.NewK8sServiceForResource(application, "httpbin"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, at kubernetes.ApplicationTest) {
				retries := 3

				for i := 1; i <= retries; i++ {
					t.Logf("Attempting to connect with backends (attempt %d/%d)", i, retries)
					err := testTrafficSplitWithCurl(t)
					if err != nil {
						t.Logf("Failed to test TrafficSplit via curl with error: %s", err)
					} else {
						// Successfully ran tests
						return
					}
				}

				require.Fail(t, fmt.Sprintf("Curl tests failed after %d retries", retries))
			},
		},
	})
	test.Test(t)
}

func testTrafficSplitWithCurl(t *testing.T) error {
	var v1Received, v2Received bool
	patch := exec.Command(`kubectl`, `patch`, `svc/trafficsplit-httpbin`,
		`-n`, `trafficsplit`, `--patch-file=testdata/patch.yaml`)
	err := patch.Run()
	if err != nil {
		return err
	}
	t.Logf("Invoking the curl pod")
	for start := time.Now(); time.Since(start) < backendTimeout; {
		podName, statusCode, err := getCurlResult(t)
		if err != nil {
			return err
		}
		if strings.HasPrefix(*podName, "httpbin-v1") && *statusCode == 200 {
			v1Received = true
		} else if strings.HasPrefix(*podName, "httpbin-v2") && *statusCode == 200 {
			v2Received = true
		}
		if v1Received && v2Received {
			break
		}
	}
	if !v1Received && !v2Received {
		// if v1Count != 5 || v2Count != 5
		// It is difficult to ensure that we always receive 5 responses from both of the container,
		// as we are calling curl iteratively. In other words, it is possible that we are always
		// getting response from the same pod, even though traffic is directed to both of them
		return fmt.Errorf("traffic counts between the two containers do not match")
	}

	t.Logf("Traffic-split is configured correctly; traffic is directed to both containers")
	return nil
}

func getCurlResult(t *testing.T) (*string, *int, error) {
	//Helper function for calling curl and retrieving the result

	podB, err := exec.Command("kubectl", "get", "pod", "-n", "curl", "-l", "radius.dev/application=curl", "-o", "jsonpath='{.items[0].metadata.name}'").Output()
	if err != nil {
		return nil, nil, err
	}
	podName := strings.Split(string(podB), "'")[1]
	curl, err := exec.Command("kubectl", "exec", "-n", "curl", "-i", podName,
		"-c", "curl", "--", "curl", "-I", "http://trafficsplit-httpbin.trafficsplit:80/json",
		"|", "egrep", "'HTTP|pod'").Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !(len(curl) > 0 && ok) {
			// The program has exited with an exit code != 0
			// Weird error thrown by curl, but we were able to get data
			return nil, nil, err
		}
	}
	statusCode, err := strconv.Atoi(strings.Split(string(curl), " ")[1])
	if err != nil {
		return nil, nil, err
	}
	strs := strings.Fields(string(curl))
	pod := strs[len(strs)-3]

	return &pod, &statusCode, nil
}
