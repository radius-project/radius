package kubernetesnamespace

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	apiv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/renderers"
)

var _ renderers.Renderer = (*noop)(nil)

const (
	application = "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app"
)

type noop struct {
}

func (r *noop) GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return nil, nil, nil
}

func (r *noop) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Return a deployment so the kubernetes namespace extension renderer can modify it
	deployment := appsv1.Deployment{}

	// Populate namespace with existing values
	if options.Application.KubernetesNamespaceOverride != nil {
		// Update deployment namespace if extension is specified
		deployment.Namespace = options.Application.KubernetesNamespaceOverride.Namespace
	} else {
		// Otherwise use the environment namespace
		deployment.Namespace = "test-env-namespace"
	}

	resources := []outputresource.OutputResource{outputresource.NewKubernetesOutputResource(resourcekinds.Deployment, outputresource.LocalIDDeployment, &deployment, deployment.ObjectMeta)}

	return renderers.RendererOutput{Resources: resources}, nil
}

type expectedNamespace struct {
	namespace string
}

type setupNamespace struct {
	baseAppKubeNamespaceExt *datamodel.KubeNamespaceOverrideExtension
}

func TestApplicationDataModelToVersioned(t *testing.T) {
	testset := []struct {
		testName          string
		expectedNamespace *expectedNamespace
		setupNamespace    *setupNamespace
		properties        datamodel.ApplicationProperties
	}{
		{
			testName:          "Test_Render_Success",
			expectedNamespace: getTestResults(false),
			setupNamespace:    getSetupNamespace(false),
			properties:        makeProperties(t, false),
		},
		{
			testName:          "Test_Render_KubeNamespaceCollision",
			expectedNamespace: getTestResults(false),
			setupNamespace:    getSetupNamespace(true),
			properties:        makeProperties(t, false),
		},
		{
			testName:          "Test_Render_NoExtension",
			expectedNamespace: getTestResults(true),
			setupNamespace:    nil,
			properties:        makeProperties(t, true),
		},
	}

	for _, tc := range testset {
		t.Run(tc.testName, func(t *testing.T) {
			renderer := &Renderer{Inner: &noop{}}
			resource := makeResource(t, tc.properties)
			dependencies := map[string]renderers.RendererDependency{}
			options := renderers.RenderOptions{Dependencies: dependencies}

			if tc.setupNamespace != nil && tc.setupNamespace.baseAppKubeNamespaceExt != nil {
				options.Application = renderers.ApplicationOptions{
					KubernetesNamespaceOverride: tc.setupNamespace.baseAppKubeNamespaceExt,
				}
			}

			output, err := renderer.Render(context.Background(), resource, options)
			require.NoError(t, err)
			require.Len(t, output.Resources, 1)

			deployment, _ := kubernetes.FindDeployment(output.Resources)
			require.NotNil(t, deployment)

			// Check deployment namespace
			if tc.expectedNamespace.namespace != "" {
				require.Equal(t, tc.expectedNamespace.namespace, deployment.Namespace)
			} else {
				require.Nil(t, deployment.Namespace)
			}
		})
	}
}

func makeResource(t *testing.T, properties datamodel.ApplicationProperties) *datamodel.Application {
	resource := datamodel.Application{
		BaseResource: v1.BaseResource{
			TrackedResource: apiv1.TrackedResource{
				ID:   application,
				Name: "test-app",
				Type: "Applications.Core/applications",
			},
		},
		Properties: properties,
	}
	return &resource
}

func makeProperties(t *testing.T, isEmpty bool) datamodel.ApplicationProperties {
	if isEmpty {
		return datamodel.ApplicationProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: application,
			},
		}
	}

	properties := datamodel.ApplicationProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: application,
		},
		Extensions: []datamodel.Extension{
			{
				Kind: datamodel.KubernetesNamespaceOverride,
				KubernetesNamespaceOverride: &datamodel.KubeNamespaceOverrideExtension{
					Namespace: "test-app-namespace",
				},
			},
		},
	}
	return properties
}

func getTestResults(envOnly bool) *expectedNamespace {
	namespace := "test-app-namespace"

	// If no extension override specified, use environment namespace
	if envOnly {
		namespace = "test-env-namespace"
	}

	return &expectedNamespace{
		namespace,
	}
}

func getSetupNamespace(hasCollision bool) *setupNamespace {
	setupNamespace := setupNamespace{}

	baseAppKubeNamespaceExt := &datamodel.KubeNamespaceOverrideExtension{
		Namespace: "test-app-namespace",
	}

	if hasCollision {
		// set up collision namespace values
		baseAppKubeNamespaceExt.Namespace = "test-app-namespace"
	}

	setupNamespace.baseAppKubeNamespaceExt = baseAppKubeNamespaceExt
	return &setupNamespace
}
