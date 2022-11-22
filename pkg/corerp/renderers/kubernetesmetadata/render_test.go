package kubernetesmetadata

import (
	"context"
	"testing"

	apiv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/renderers"
)

var _ renderers.Renderer = (*noop)(nil)

const (
	application = "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app"
	container   = "/subscriptions/test-sub-id/resourceGroups/test-group/providers/Applications.Core/containers/test-container"
)

type noop struct {
}

func (r *noop) GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return nil, nil, nil
}

func (r *noop) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Return a deployment so the kubernetes metadata extension renderer can modify it
	deployment := appsv1.Deployment{}

	// Populate Meta labels with existing values
	deployment.Annotations = map[string]string{"prior.MetaAnnotation1": "prior.MetaAnnotationVal1", "prior.MetaAnnotation2": "prior.MetaAnnotationVal2"}
	deployment.Labels = map[string]string{"prior.MetaLabel1": "prior.MetaLabelVal1", "prior.MetaLabel2": "prior.MetaLabelVal2"}

	resources := []outputresource.OutputResource{outputresource.NewKubernetesOutputResource(resourcekinds.Deployment, outputresource.LocalIDDeployment, &deployment, deployment.ObjectMeta)}

	return renderers.RendererOutput{Resources: resources}, nil
}

func Test_Render_Success(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	// Get expected values
	metaAnn, metaLbl, specAnn, specLbl := getTestResultMaps()

	properties := makeProperties(t)
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	// Check Meta Labels
	require.Equal(t, metaAnn, deployment.Annotations)
	require.Equal(t, metaLbl, deployment.Labels)

	// Check Spec Labels
	require.Equal(t, specAnn, deployment.Spec.Template.Annotations)
	require.Equal(t, specLbl, deployment.Spec.Template.Labels)
}

func Test_Render_CascadeKubeMetadata(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	// Get expected values
	metaAnn, metaLbl, specAnn, specLbl, baseEnvKubeMetadataExt, baseAppKubeMetadataExt := getCascadeTestResultMaps(false)

	properties := makeProperties(t)
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	options := renderers.RenderOptions{Dependencies: dependencies}

	options.Environment = renderers.EnvironmentOptions{
		KubernetesMetadata: &baseEnvKubeMetadataExt,
	}
	options.Application = renderers.ApplicationOptions{
		KubernetesMetadata: &baseAppKubeMetadataExt,
	}
	output, err := renderer.Render(context.Background(), resource, options)
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	// Check Meta Labels
	require.Equal(t, metaAnn, deployment.Annotations)
	require.Equal(t, metaLbl, deployment.Labels)

	// Check Spec Labels
	require.Equal(t, specAnn, deployment.Spec.Template.Annotations)
	require.Equal(t, specLbl, deployment.Spec.Template.Labels)
}

func Test_Render_KubeMetadataCollision(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	// Get expected values
	metaAnn, metaLbl, specAnn, specLbl, baseEnvKubeMetadataExt, baseAppKubeMetadataExt := getCascadeTestResultMaps(true)

	properties := makeProperties(t)
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	options := renderers.RenderOptions{Dependencies: dependencies}

	// Set Environment KubernetesMetadataExtension
	options.Environment = renderers.EnvironmentOptions{
		KubernetesMetadata: &baseEnvKubeMetadataExt,
	}

	// Set Application KubernetesMetadataExtension
	options.Application = renderers.ApplicationOptions{
		KubernetesMetadata: &baseAppKubeMetadataExt,
	}
	output, err := renderer.Render(context.Background(), resource, options)
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	// Check Meta Labels
	require.Equal(t, metaAnn, deployment.Annotations)
	require.Equal(t, metaLbl, deployment.Labels)

	// Check Spec Labels
	require.Equal(t, specAnn, deployment.Spec.Template.Annotations)
	require.Equal(t, specLbl, deployment.Spec.Template.Labels)
}

func Test_Render_OnlyAppExtension(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	// Get expected values
	metaAnn, metaLbl, specAnn, specLbl, baseAppKubeMetadataExt := getOnlyAppExtTestResultMaps()

	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: application,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
	}

	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	options := renderers.RenderOptions{Dependencies: dependencies}

	// Set Application KubernetesMetadataExtension
	options.Application = renderers.ApplicationOptions{
		KubernetesMetadata: &baseAppKubeMetadataExt,
	}

	output, err := renderer.Render(context.Background(), resource, options)
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	// Check Meta Labels
	require.Equal(t, metaAnn, deployment.Annotations)
	require.Equal(t, metaLbl, deployment.Labels)

	// Check Spec Labels
	require.Equal(t, specAnn, deployment.Spec.Template.Annotations)
	require.Equal(t, specLbl, deployment.Spec.Template.Labels)
}

func Test_Render_NoExtension(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: application,
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
	}

	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	ann := map[string]string{"prior.MetaAnnotation1": "prior.MetaAnnotationVal1", "prior.MetaAnnotation2": "prior.MetaAnnotationVal2"}
	lbl := map[string]string{"prior.MetaLabel1": "prior.MetaLabelVal1", "prior.MetaLabel2": "prior.MetaLabelVal2"}

	output, err := renderer.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	// Check Spec Labels
	require.Equal(t, ann, deployment.Annotations)
	require.Equal(t, lbl, deployment.Labels)

	// Check Meta Labels
	require.Nil(t, deployment.Spec.Template.Annotations)
	require.Nil(t, deployment.Spec.Template.Labels)
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

func makeProperties(t *testing.T) datamodel.ContainerProperties {
	var (
		replicas  int32  = 3
		preplicas *int32 = &replicas
	)

	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
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
	return properties
}

func getTestResultMaps() (map[string]string, map[string]string, map[string]string, map[string]string) {
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

	return metaAnn, metaLbl, specAnn, specLbl
}

func getCascadeTestResultMaps(hasCollision bool) (map[string]string, map[string]string, map[string]string, map[string]string, datamodel.KubeMetadataExtension, datamodel.KubeMetadataExtension) {
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

	baseEnvKubeMetadataExt := datamodel.KubeMetadataExtension{
		Annotations: map[string]string{
			"env.ann1": "env.annval1",
			"env.ann2": "env.annval2",
		},
		Labels: map[string]string{
			"env.lbl1": "env.lblval1",
			"env.lbl2": "env.lblval2",
		},
	}
	baseAppKubeMetadataExt := datamodel.KubeMetadataExtension{
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

	return metaAnn, metaLbl, specAnn, specLbl, baseEnvKubeMetadataExt, baseAppKubeMetadataExt
}

func getOnlyAppExtTestResultMaps() (map[string]string, map[string]string, map[string]string, map[string]string, datamodel.KubeMetadataExtension) {
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

	baseAppKubeMetadataExt := datamodel.KubeMetadataExtension{
		Annotations: map[string]string{
			"app.ann1": "app.annval1",
			"app.ann2": "app.annval2",
		},
		Labels: map[string]string{
			"app.lbl1": "app.lblval1",
			"app.lbl2": "app.lblval2",
		},
	}

	return metaAnn, metaLbl, specAnn, specLbl, baseAppKubeMetadataExt
}
