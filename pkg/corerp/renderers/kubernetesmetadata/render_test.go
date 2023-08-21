package kubernetesmetadata

import (
	"context"
	"testing"

	apiv1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/resourcekinds"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

var _ renderers.Renderer = (*noop)(nil)

const (
	application = "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app"
	container   = "/subscriptions/test-sub-id/resourceGroups/test-group/providers/Applications.Core/containers/test-container"
)

type noop struct {
}

func (r *noop) GetDependencyIDs(ctx context.Context, resource v1.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return nil, nil, nil
}

func (r *noop) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Return a deployment so the kubernetes metadata extension renderer can modify it
	deployment := appsv1.Deployment{}

	// Populate Meta labels with existing values
	deployment.Annotations = map[string]string{"prior.MetaAnnotation1": "prior.MetaAnnotationVal1", "prior.MetaAnnotation2": "prior.MetaAnnotationVal2"}
	deployment.Labels = map[string]string{"prior.MetaLabel1": "prior.MetaLabelVal1", "prior.MetaLabel2": "prior.MetaLabelVal2"}

	resources := []rpv1.OutputResource{rpv1.NewKubernetesOutputResource(resourcekinds.Deployment, rpv1.LocalIDDeployment, &deployment, deployment.ObjectMeta)}

	return renderers.RendererOutput{Resources: resources}, nil
}

type expectedMaps struct {
	metaAnn map[string]string
	metaLbl map[string]string
	specAnn map[string]string
	specLbl map[string]string
}

type setupMaps struct {
	baseEnvKubeMetadataExt *datamodel.KubeMetadataExtension
	baseAppKubeMetadataExt *datamodel.KubeMetadataExtension
}

func TestApplicationDataModelToVersioned(t *testing.T) {
	testset := []struct {
		testName     string
		expectedMaps *expectedMaps
		setupMaps    *setupMaps
		properties   datamodel.ContainerProperties
	}{
		{
			testName:     "Test_Render_Success",
			expectedMaps: getTestResultMaps(),
			setupMaps:    nil,
			properties:   makeProperties(t, false, false),
		},
		{
			testName:     "Test_Render_CascadeKubeMetadata",
			expectedMaps: getCascadeTestResultMaps(),
			setupMaps:    getSetUpMaps(false, false),
			properties:   makeProperties(t, false, false),
		},
		{
			testName:     "Test_Render_KubeMetadataCollision",
			expectedMaps: getCascadeTestResultMaps(),
			setupMaps:    getSetUpMaps(true, false),
			properties:   makeProperties(t, false, false),
		},
		{
			testName:     "Test_Render_OnlyAppExtension",
			expectedMaps: getOnlyAppTestResultMaps(),
			setupMaps:    getSetUpMaps(false, true),
			properties:   makeProperties(t, true, false),
		},
		{
			testName:     "Test_Render_NoExtension",
			expectedMaps: getEmptyTestResultMaps(),
			setupMaps:    nil,
			properties:   makeProperties(t, true, false),
		},
		{
			testName:     "Test_ReserveKey_Collision",
			expectedMaps: getTestResultMaps(),
			setupMaps:    nil,
			properties:   makeProperties(t, false, true),
		},
	}

	for _, tc := range testset {
		t.Run(tc.testName, func(t *testing.T) {
			renderer := &Renderer{Inner: &noop{}}
			resource := makeResource(t, tc.properties)
			dependencies := map[string]renderers.RendererDependency{}
			options := renderers.RenderOptions{Dependencies: dependencies}

			if tc.setupMaps != nil && tc.setupMaps.baseEnvKubeMetadataExt != nil {
				options.Environment = renderers.EnvironmentOptions{
					KubernetesMetadata: tc.setupMaps.baseEnvKubeMetadataExt,
				}
			}

			if tc.setupMaps != nil && tc.setupMaps.baseAppKubeMetadataExt != nil {
				options.Application = renderers.ApplicationOptions{
					KubernetesMetadata: tc.setupMaps.baseAppKubeMetadataExt,
				}
			}

			output, err := renderer.Render(context.Background(), resource, options)
			require.NoError(t, err)
			require.Len(t, output.Resources, 1)

			deployment, _ := kubernetes.FindDeployment(output.Resources)
			require.NotNil(t, deployment)

			// Check Meta Labels
			require.Equal(t, tc.expectedMaps.metaAnn, deployment.Annotations)
			require.Equal(t, tc.expectedMaps.metaLbl, deployment.Labels)

			// Check Spec Labels
			if tc.expectedMaps.specAnn != nil {
				require.Equal(t, tc.expectedMaps.specAnn, deployment.Spec.Template.Annotations)
				require.Equal(t, tc.expectedMaps.specLbl, deployment.Spec.Template.Labels)
			} else {
				require.Nil(t, deployment.Spec.Template.Annotations)
				require.Nil(t, deployment.Spec.Template.Labels)
			}
		})
	}
}

func makeResource(t *testing.T, properties datamodel.ContainerProperties) *datamodel.ContainerResource {
	resource := datamodel.ContainerResource{
		BaseResource: v1.BaseResource{
			TrackedResource: apiv1.TrackedResource{
				ID:   container,
				Name: "test-container",
				Type: "Applications.Core/containers",
			},
		},
		Properties: properties,
	}
	return &resource
}

func makeProperties(t *testing.T, isEmpty bool, hasReservedKey bool) datamodel.ContainerProperties {
	if isEmpty {
		return datamodel.ContainerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: application,
			},
			Container: datamodel.Container{
				Image: "someimage:latest",
			},
		}
	}

	var (
		replicas  int32  = 3
		preplicas *int32 = &replicas
	)

	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: application,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
		Extensions: []datamodel.Extension{
			{
				Kind: datamodel.KubernetesMetadata,
				KubernetesMetadata: &datamodel.KubeMetadataExtension{
					Annotations: map[string]string{
						"test.ann1": "ann1.val",
						"test.ann2": "ann1.val",
						"test.ann3": "ann1.val",
					},
					Labels: map[string]string{
						"test.lbl1": "lbl1.val",
						"test.lbl2": "lbl2.val",
						"test.lbl3": "lbl3.val",
					},
				},
			},
			{
				Kind: datamodel.ManualScaling,
				ManualScaling: &datamodel.ManualScalingExtension{
					Replicas: preplicas,
				},
			},
		},
	}

	if hasReservedKey {
		properties.Extensions[0].KubernetesMetadata.Annotations["radius.dev/testannkey"] = "radius.dev/testannval"
		properties.Extensions[0].KubernetesMetadata.Labels["radius.dev/testlblkey"] = "radius.dev/testlblval"
	}

	return properties
}

func getTestResultMaps() *expectedMaps {
	metaAnn := map[string]string{
		"test.ann1":             "ann1.val",
		"test.ann2":             "ann1.val",
		"test.ann3":             "ann1.val",
		"prior.MetaAnnotation1": "prior.MetaAnnotationVal1",
		"prior.MetaAnnotation2": "prior.MetaAnnotationVal2",
	}
	metaLbl := map[string]string{
		"test.lbl1":        "lbl1.val",
		"test.lbl2":        "lbl2.val",
		"test.lbl3":        "lbl3.val",
		"prior.MetaLabel1": "prior.MetaLabelVal1",
		"prior.MetaLabel2": "prior.MetaLabelVal2",
	}
	specAnn := map[string]string{
		"test.ann1": "ann1.val",
		"test.ann2": "ann1.val",
		"test.ann3": "ann1.val",
	}
	specLbl := map[string]string{
		"test.lbl1": "lbl1.val",
		"test.lbl2": "lbl2.val",
		"test.lbl3": "lbl3.val",
	}

	return &expectedMaps{
		metaAnn,
		metaLbl,
		specAnn,
		specLbl,
	}
}

func getEmptyTestResultMaps() *expectedMaps {
	metaAnn := map[string]string{
		"prior.MetaAnnotation1": "prior.MetaAnnotationVal1",
		"prior.MetaAnnotation2": "prior.MetaAnnotationVal2",
	}
	metaLbl := map[string]string{
		"prior.MetaLabel1": "prior.MetaLabelVal1",
		"prior.MetaLabel2": "prior.MetaLabelVal2",
	}

	return &expectedMaps{
		metaAnn,
		metaLbl,
		nil,
		nil,
	}
}

func getCascadeTestResultMaps() *expectedMaps {
	metaAnn := map[string]string{
		"env.ann1":              "env.annval1",
		"env.ann2":              "env.annval2",
		"app.ann1":              "app.annval1",
		"app.ann2":              "app.annval2",
		"test.ann1":             "ann1.val",
		"test.ann2":             "ann1.val",
		"test.ann3":             "ann1.val",
		"prior.MetaAnnotation1": "prior.MetaAnnotationVal1",
		"prior.MetaAnnotation2": "prior.MetaAnnotationVal2",
	}
	metaLbl := map[string]string{
		"env.lbl1":         "env.lblval1",
		"env.lbl2":         "env.lblval2",
		"app.lbl1":         "app.lblval1",
		"app.lbl2":         "app.lblval2",
		"test.lbl1":        "lbl1.val",
		"test.lbl2":        "lbl2.val",
		"test.lbl3":        "lbl3.val",
		"prior.MetaLabel1": "prior.MetaLabelVal1",
		"prior.MetaLabel2": "prior.MetaLabelVal2",
	}
	specAnn := map[string]string{
		"env.ann1":  "env.annval1",
		"env.ann2":  "env.annval2",
		"app.ann1":  "app.annval1",
		"app.ann2":  "app.annval2",
		"test.ann1": "ann1.val",
		"test.ann2": "ann1.val",
		"test.ann3": "ann1.val",
	}
	specLbl := map[string]string{
		"env.lbl1":  "env.lblval1",
		"env.lbl2":  "env.lblval2",
		"app.lbl1":  "app.lblval1",
		"app.lbl2":  "app.lblval2",
		"test.lbl1": "lbl1.val",
		"test.lbl2": "lbl2.val",
		"test.lbl3": "lbl3.val",
	}

	return &expectedMaps{
		metaAnn: metaAnn,
		metaLbl: metaLbl,
		specAnn: specAnn,
		specLbl: specLbl,
	}

}

func getSetUpMaps(hasCollision bool, appOnly bool) *setupMaps {
	setupMap := setupMaps{}

	baseEnvKubeMetadataExt := &datamodel.KubeMetadataExtension{
		Annotations: map[string]string{
			"env.ann1": "env.annval1",
			"env.ann2": "env.annval2",
		},
		Labels: map[string]string{
			"env.lbl1": "env.lblval1",
			"env.lbl2": "env.lblval2",
		},
	}
	baseAppKubeMetadataExt := &datamodel.KubeMetadataExtension{
		Annotations: map[string]string{
			"app.ann1": "app.annval1",
			"app.ann2": "app.annval2",
		},
		Labels: map[string]string{
			"app.lbl1": "app.lblval1",
			"app.lbl2": "app.lblval2",
		},
	}

	if hasCollision {
		//set up collsion values
		baseEnvKubeMetadataExt.Annotations["test.ann1"] = "env-annotation-collsion"
		baseEnvKubeMetadataExt.Labels["test.lbl1"] = "env-label-collsion"
		baseAppKubeMetadataExt.Annotations["test.ann1"] = "app-annotation-collsion"
		baseAppKubeMetadataExt.Labels["test.lbl1"] = "app-label-collsion"
	}

	setupMap.baseAppKubeMetadataExt = baseAppKubeMetadataExt

	if !appOnly {
		setupMap.baseEnvKubeMetadataExt = baseEnvKubeMetadataExt
	}

	return &setupMap
}

func getOnlyAppTestResultMaps() *expectedMaps {
	metaAnn := map[string]string{
		"app.ann1":              "app.annval1",
		"app.ann2":              "app.annval2",
		"prior.MetaAnnotation1": "prior.MetaAnnotationVal1",
		"prior.MetaAnnotation2": "prior.MetaAnnotationVal2",
	}
	metaLbl := map[string]string{
		"app.lbl1":         "app.lblval1",
		"app.lbl2":         "app.lblval2",
		"prior.MetaLabel1": "prior.MetaLabelVal1",
		"prior.MetaLabel2": "prior.MetaLabelVal2",
	}
	specAnn := map[string]string{
		"app.ann1": "app.annval1",
		"app.ann2": "app.annval2",
	}
	specLbl := map[string]string{
		"app.lbl1": "app.lblval1",
		"app.lbl2": "app.lblval2",
	}

	return &expectedMaps{
		metaAnn,
		metaLbl,
		specAnn,
		specLbl,
	}
}
