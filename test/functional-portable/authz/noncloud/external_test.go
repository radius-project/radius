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

package authz

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/rogue"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func installedExternalProvider(t *testing.T) bool {
	t.Helper()
	options := test.NewTestOptions(t)
	deployment, err := options.K8sClient.AppsV1().Deployments(radiusNamespace).Get(t.Context(), "ucp", metav1.GetOptions{})
	require.NoError(t, err)
	var name string
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Name == "config-volume" && volume.ConfigMap != nil {
			name = volume.ConfigMap.Name
		}
	}
	require.NotEmpty(t, name, "UCP must reference its active configuration generation")
	config, err := options.K8sClient.CoreV1().ConfigMaps(radiusNamespace).Get(t.Context(), name, metav1.GetOptions{})
	require.NoError(t, err)
	var parsed struct {
		Authorization authz.Options `yaml:"authorization"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(config.Data["ucp-config.yaml"]), &parsed))
	return parsed.Authorization.EffectiveMTLSProvider() == "external"
}

func testExternalModeTokenReplay(t *testing.T) {
	if !installedExternalProvider(t) {
		t.Skip("requires an external-provider installation with a paired engine and operator encryption")
	}
	if expectedInstalledMode(t) != authz.ModeEnforce {
		t.Skip("replay rejection requires Enforce; DryRun observations are covered by local HTTP tests")
	}
	options := test.NewTestOptions(t)
	data := map[string][]byte{}
	for _, receiver := range []string{"applications-rp", "ucp"} {
		token, err := options.K8sClient.CoreV1().ServiceAccounts(radiusNamespace).CreateToken(t.Context(), "ucp", &authenticationv1.TokenRequest{
			Spec: authenticationv1.TokenRequestSpec{Audiences: []string{receiver + "." + radiusNamespace + ".svc"}, ExpirationSeconds: to.Ptr(int64(600))},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
		data[receiver] = []byte(token.Status.Token)
	}
	namespace := createNamespace(t, options.K8sClient)
	secret, err := options.K8sClient.CoreV1().Secrets(namespace).Create(t.Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "receiver-tokens"}, Data: data,
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	client := rogue.New(t, options.K8sClient, options.K8sConfig, rogue.Options{Namespace: namespace, TokenSecretName: secret.Name})
	endpoint := strings.Replace(applicationsRPOperationsURL, "https://", "http://", 1)
	for _, tt := range []struct {
		token  string
		status int
	}{{"applications-rp", http.StatusOK}, {"ucp", http.StatusUnauthorized}} {
		response, err := client.Do(t.Context(), rogue.Request{
			URL: endpoint, Token: tt.token, Headers: map[string]string{"X-Forwarded-Client-Cert": "forged", "X-Remote-User": "admin"},
		})
		require.NoError(t, err)
		require.Equal(t, tt.status, response.StatusCode)
		if tt.status != http.StatusOK {
			var body v1.ErrorResponse
			require.NoError(t, json.Unmarshal([]byte(response.Body), &body))
			require.Equal(t, v1.CodeInvalidAuthenticationInfo, body.Error.Code)
		}
	}
}
