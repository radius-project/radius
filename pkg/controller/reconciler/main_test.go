/*
Copyright 2023.

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

package reconciler

import (
	"os"
	"path/filepath"
	"testing"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// config holds a reference to the rest config for the test environment.
var config *rest.Config

// scheme holds a reference to the scheme for the test environment.
var scheme *runtime.Scheme

// testOptions holds a reference to the webhook install options for the test environment.
var testOptions *testWebhookOptions

type testWebhookOptions struct {
	LocalServingHost    string
	LocalServingPort    int
	LocalServingCertDir string
}

// TestMain will be called before running any tests in the package.
//
// We're using this to ensure that one (and only one) copy of env-test is booted up.
func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// We don't know how to start the envtest environment. Just go ahead and call the tests so they can skip.
		os.Exit(m.Run()) //nolint:forbidigo // this is OK inside the TestMain function.
		return
	}

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "deploy", "Chart", "crds", "radius")},
		ErrorIfCRDPathMissing: true,
	}

	initializeWebhookInEnvironment(env)
	cfg, err := env.Start()
	if err != nil {
		panic("failed to start envtest" + err.Error())
	}

	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = radappiov1alpha3.AddToScheme(s)

	config = cfg
	scheme = s
	testOptions = &testWebhookOptions{
		LocalServingHost:    env.WebhookInstallOptions.LocalServingHost,
		LocalServingPort:    env.WebhookInstallOptions.LocalServingPort,
		LocalServingCertDir: env.WebhookInstallOptions.LocalServingCertDir,
	}

	exitCode := m.Run()

	// Note: CANNOT use defer here because os.Exit won't run defers.
	err = env.Stop()
	if err != nil {
		panic("failed to stop envtest" + err.Error())
	}

	os.Exit(exitCode) //nolint:forbidigo // this is OK inside the TestMain function.
}

func SkipWithoutEnvironment(t *testing.T) {
	if config == nil {
		t.Skip("Skipping test because envtest could not be started. Running `make test` will run tests with the correct setting.")
		return
	}
}

// initializeWebhookInEnvironment initializes the webhook installation options and validating configuration  in the given environment for validating webhooks.
func initializeWebhookInEnvironment(env *envtest.Environment) {
	namespacedScopeV1 := admissionv1.NamespacedScope
	failedTypeV1 := admissionv1.Ignore
	equivalentTypeV1 := admissionv1.Equivalent
	noSideEffectsV1 := admissionv1.SideEffectClassNone
	recipeWebhookPathV1 := "/validate-radapp-io-v1alpha3-recipe"
	deploymentWebhookPathV1 := "/mutate-apps-v1-deployment"

	env.WebhookInstallOptions = envtest.WebhookInstallOptions{
		ValidatingWebhooks: []*admissionv1.ValidatingWebhookConfiguration{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "recipe-webhook-config",
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "ValidatingWebhookConfiguration",
					APIVersion: "admissionregistration.k8s.io/v1",
				},
				Webhooks: []admissionv1.ValidatingWebhook{
					{
						Name: "recipe-webhook.radapp.io",
						Rules: []admissionv1.RuleWithOperations{
							{
								Operations: []admissionv1.OperationType{"CREATE", "UPDATE"},
								Rule: admissionv1.Rule{
									APIGroups:   []string{"radapp.io"},
									APIVersions: []string{"v1alpha3"},
									Resources:   []string{"recipes"},
									Scope:       &namespacedScopeV1,
								},
							},
						},
						FailurePolicy: &failedTypeV1,
						MatchPolicy:   &equivalentTypeV1,
						SideEffects:   &noSideEffectsV1,
						ClientConfig: admissionv1.WebhookClientConfig{
							Service: &admissionv1.ServiceReference{
								Name:      "controller",
								Namespace: "default",
								Path:      &recipeWebhookPathV1,
							},
						},
						AdmissionReviewVersions: []string{"v1"},
					},
				},
			},
		},
		MutatingWebhooks: []*admissionv1.MutatingWebhookConfiguration{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment-webhook-config",
				},
				TypeMeta: metav1.TypeMeta{
					Kind:       "MutatingWebhookConfiguration",
					APIVersion: "admissionregistration.k8s.io/v1",
				},
				Webhooks: []admissionv1.MutatingWebhook{
					{
						Name: "deployment-webhook.apps.io",
						Rules: []admissionv1.RuleWithOperations{
							{
								Operations: []admissionv1.OperationType{"CREATE", "UPDATE"},
								Rule: admissionv1.Rule{
									APIGroups:   []string{"apps"},
									APIVersions: []string{"v1"},
									Resources:   []string{"deployments"},
									Scope:       &namespacedScopeV1,
								},
							},
						},
						FailurePolicy: &failedTypeV1,
						MatchPolicy:   &equivalentTypeV1,
						SideEffects:   &noSideEffectsV1,
						ClientConfig: admissionv1.WebhookClientConfig{
							Service: &admissionv1.ServiceReference{
								Name:      "controller",
								Namespace: "default",
								Path:      &deploymentWebhookPathV1,
							},
						},
						AdmissionReviewVersions: []string{"v1"},
					},
				},
			},
		},
	}
}
