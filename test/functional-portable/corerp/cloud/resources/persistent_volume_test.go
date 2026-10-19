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

package resource_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v3"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8s "k8s.io/client-go/kubernetes"
	"uuid"
)

func Test_PersistentVolume_AzureKeyVault(t *testing.T) {
	const (
		name          = "corerp-azure-keyvault-volume"
		containerName = "volume-consumer"
		volumeName    = "keyvault-volume"
		mountPath     = "/var/secrets"
		secretFile    = "secret-value"
	)

	subscriptionID := requiredCloudEnv(t, "AZURE_SUBSCRIPTION_ID")
	resourceGroup := requiredCloudEnv(t, "INTEGRATION_TEST_RESOURCE_GROUP_NAME")
	azureScope := fmt.Sprintf("azureScope=/subscriptions/%s/resourceGroups/%s", subscriptionID, resourceGroup)
	keyVaultName := "radcsi" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	test := rp.NewRPTest(t, name, nil)
	oidcIssuer := getKubernetesOIDCIssuer(t.Context(), t, test.Options.K8sClient)
	resourcesClient, err := armresources.NewClient(subscriptionID, azureTestCredential(t.Context(), t), nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Minute)
		defer cancel()

		poller, err := resourcesClient.BeginDelete(
			ctx,
			resourceGroup,
			"Microsoft.KeyVault",
			"",
			"vaults",
			keyVaultName,
			"2023-07-01",
			nil,
		)
		if isNotFoundResponse(err) {
			return
		}
		if err == nil {
			_, err = poller.PollUntilDone(ctx, nil)
		}
		if err != nil {
			t.Errorf("delete test Azure Key Vault %s: %v", keyVaultName, err)
		}
	})

	test.Steps = []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(
				"testdata/corerp-resources-volume-azure-keyvault.bicep",
				testutil.GetMagpieImage(),
				"oidcIssuer="+oidcIssuer,
				azureScope,
				"keyVaultName="+keyVaultName,
			),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name + "-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: containerName,
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: volumeName,
						Type: validation.VolumesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, containerName),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				secretProviderClass, err := test.Options.DynamicClient.Resource(schema.GroupVersionResource{
					Group:    "secrets-store.csi.x-k8s.io",
					Version:  "v1",
					Resource: "secretproviderclasses",
				}).Namespace(name).Get(ctx, volumeName, metav1.GetOptions{})
				require.NoError(t, err)
				provider, found, err := unstructured.NestedString(secretProviderClass.Object, "spec", "provider")
				require.NoError(t, err)
				require.True(t, found)
				require.Equal(t, "azure", provider)
				parameters, found, err := unstructured.NestedStringMap(secretProviderClass.Object, "spec", "parameters")
				require.NoError(t, err)
				require.True(t, found)
				require.NotEmpty(t, parameters["clientID"])
				require.NotEmpty(t, parameters["tenantID"])
				require.Contains(t, parameters["objects"], "objectName: csi-test-secret")
				require.Contains(t, parameters["objects"], "objectAlias: "+secretFile)

				deployment, err := test.Options.K8sClient.AppsV1().Deployments(name).Get(ctx, containerName, metav1.GetOptions{})
				require.NoError(t, err)
				require.Len(t, deployment.Spec.Template.Spec.Volumes, 1)
				require.NotNil(t, deployment.Spec.Template.Spec.Volumes[0].CSI)
				require.Equal(t, "secrets-store.csi.k8s.io", deployment.Spec.Template.Spec.Volumes[0].CSI.Driver)
				require.Equal(t, volumeName, deployment.Spec.Template.Spec.Volumes[0].CSI.VolumeAttributes["secretProviderClass"])
				require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
				require.Contains(t, deployment.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
					Name:      "secrets",
					MountPath: mountPath,
					ReadOnly:  true,
				})

				pods, err := test.Options.K8sClient.CoreV1().Pods(name).List(ctx, metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(kubernetes.MakeSelectorLabels(name, containerName)).String(),
				})
				require.NoError(t, err)
				require.Len(t, pods.Items, 1)

				require.EventuallyWithT(t, func(collect *assert.CollectT) {
					logs, err := testutil.GetPodLogs(ctx, test.Options.K8sClient, name, pods.Items[0].Name, containerName)
					assert.NoError(collect, err)
					assert.Contains(collect, logs, "csi-secret-mounted")
				}, 30*time.Second, time.Second)
			},
		},
	}

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureCSIDriver, rp.FeatureAzure}
	test.Test(t)
}

func getKubernetesOIDCIssuer(ctx context.Context, t *testing.T, client k8s.Interface) string {
	t.Helper()

	data, err := client.Discovery().RESTClient().Get().AbsPath("/.well-known/openid-configuration").DoRaw(ctx)
	require.NoError(t, err)

	config := struct {
		Issuer string `json:"issuer"`
	}{}
	require.NoError(t, json.Unmarshal(data, &config))
	require.NotEmpty(t, config.Issuer)
	return config.Issuer
}
