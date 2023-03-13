package httproute

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	applicationName = "test-application"
	resourceName    = "test-route"
	applicationPath = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Applications.Core/applications/"
)

type setupMaps struct {
	envKubeMetadataExt *datamodel.KubeMetadataExtension
	appKubeMetadataExt *datamodel.KubeMetadataExtension
}

type expectedMaps struct {
	metaAnn map[string]string
	metaLbl map[string]string
}

func createContext(t *testing.T) context.Context {
	logger, err := ucplog.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}

	return logr.NewContext(context.Background(), logger)
}

func TestHTTPRouteRenderer(t *testing.T) {
	tests := []struct {
		name         string
		port         int32
		options      renderers.RenderOptions
		setupMaps    *setupMaps
		expectedMaps *expectedMaps
	}{
		{
			name:         "WithPort",
			port:         6379,
			options:      getRenderOptions(0),
			setupMaps:    nil,
			expectedMaps: nil,
		},
		{
			name:         "WithDefaultPort",
			port:         renderers.DefaultPort,
			options:      getRenderOptions(0),
			setupMaps:    nil,
			expectedMaps: nil,
		},
		{
			name:         "WithEnvKME",
			port:         renderers.DefaultPort,
			options:      getRenderOptions(1),
			setupMaps:    getSetUpMaps(true),
			expectedMaps: getExpectedMaps(true),
		},
		{
			name:         "WithEnvAppKME",
			port:         renderers.DefaultPort,
			options:      getRenderOptions(2),
			setupMaps:    getSetUpMaps(false),
			expectedMaps: getExpectedMaps(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Renderer{}

			properties := makeHTTPRouteProperties(tt.port)
			resource := makeResource(t, &properties)

			output, err := r.Render(context.Background(), resource, tt.options)
			require.NoError(t, err)
			require.Len(t, output.Resources, 1)
			require.Empty(t, output.SecretValues)

			expectedServicePort := corev1.ServicePort{
				Name:       resourceName,
				Port:       tt.port,
				TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(ResourceTypeSuffix + resource.Name)),
				Protocol:   "TCP",
			}
			expectedValues := map[string]rpv1.ComputedValueReference{
				"hostname": {Value: kubernetes.NormalizeResourceName(resourceName)},
				"port":     {Value: tt.port},
				"scheme":   {Value: "http"},
				"url":      {Value: fmt.Sprintf("http://%s:%d", kubernetes.NormalizeResourceName(resourceName), tt.port)},
			}

			require.Equal(t, expectedValues, output.ComputedValues)

			service, outputResource := kubernetes.FindService(output.Resources)
			expectedOutputResource := rpv1.NewKubernetesOutputResource(resourcekinds.Service, rpv1.LocalIDService, service, service.ObjectMeta)

			require.Equal(t, expectedOutputResource, outputResource)
			require.Equal(t, kubernetes.NormalizeResourceName(resource.Name), service.Name)
			require.Equal(t, "", service.Namespace)
			require.Equal(t, kubernetes.MakeRouteSelectorLabels(applicationName, ResourceTypeSuffix, resourceName), service.Spec.Selector)
			require.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)
			require.Len(t, service.Spec.Ports, 1)

			servicePort := service.Spec.Ports[0]
			require.Equal(t, expectedServicePort, servicePort)

			// Check values of labels and annotations
			if tt.expectedMaps != nil {
				require.Equal(t, tt.expectedMaps.metaAnn, service.Annotations)
				require.Equal(t, tt.expectedMaps.metaLbl, service.Labels)
			} else {
				require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName, resource.ResourceTypeName()), service.Labels)
			}
		})
	}
}

func Test_GetDependencyIDs_Empty(t *testing.T) {
	r := &Renderer{}
	dependencies, _, err := r.GetDependencyIDs(createContext(t), &datamodel.HTTPRoute{})
	require.NoError(t, err)
	require.Empty(t, dependencies)
}

func makeHTTPRouteProperties(port int32) datamodel.HTTPRouteProperties {
	properties := datamodel.HTTPRouteProperties{}
	str := []string{applicationPath, applicationName}
	properties.Application = strings.Join(str, "")
	if port > 0 {
		properties.Port = port
	}

	return properties
}

func makeResource(t *testing.T, properties *datamodel.HTTPRouteProperties) *datamodel.HTTPRoute {
	dm := datamodel.HTTPRoute{Properties: properties}
	dm.Name = resourceName

	return &dm
}

func getRenderOptions(opt int) renderers.RenderOptions {
	/*
		opt: 0 - no KubeMetadata
		opt: 1 - env KubeMetadata
		opt: 2 - env and app KubeMetadata
	*/

	dependencies := map[string]renderers.RendererDependency{}
	option := renderers.RenderOptions{Dependencies: dependencies}
	if opt == 0 {
		return option
	}

	option.Environment = renderers.EnvironmentOptions{
		KubernetesMetadata: &datamodel.KubeMetadataExtension{
			Annotations: getSetUpMaps(true).envKubeMetadataExt.Annotations,
			Labels:      getSetUpMaps(true).envKubeMetadataExt.Labels,
		}}

	if opt == 2 {
		option.Application = renderers.ApplicationOptions{
			KubernetesMetadata: &datamodel.KubeMetadataExtension{
				Annotations: getSetUpMaps(false).appKubeMetadataExt.Annotations,
				Labels:      getSetUpMaps(false).appKubeMetadataExt.Labels,
			}}
	}

	return option
}

func getSetUpMaps(envOnly bool) *setupMaps {
	setupMap := setupMaps{}

	envKubeMetadataExt := &datamodel.KubeMetadataExtension{
		Annotations: map[string]string{
			"env.ann1":  "env.annval1",
			"env.ann2":  "env.annval2",
			"test.ann1": "env.annval1",
		},
		Labels: map[string]string{
			"env.lbl1":  "env.lblval1",
			"env.lbl2":  "env.lblval2",
			"test.lbl1": "env.lblval1",
		},
	}
	appKubeMetadataExt := &datamodel.KubeMetadataExtension{
		Annotations: map[string]string{
			"app.ann1":  "app.annval1",
			"app.ann2":  "app.annval2",
			"test.ann1": "override.app.annval1",
		},
		Labels: map[string]string{
			"app.lbl1":  "app.lblval1",
			"app.lbl2":  "app.lblval2",
			"test.lbl1": "override.app.lblval1",
		},
	}

	setupMap.envKubeMetadataExt = envKubeMetadataExt

	if !envOnly {
		setupMap.appKubeMetadataExt = appKubeMetadataExt
	}

	return &setupMap
}

func getExpectedMaps(envOnly bool) *expectedMaps {
	metaAnn := map[string]string{
		"env.ann1":  "env.annval1",
		"env.ann2":  "env.annval2",
		"test.ann1": "env.annval1",
	}
	metaLbl := map[string]string{
		"env.lbl1":                     "env.lblval1",
		"env.lbl2":                     "env.lblval2",
		"test.lbl1":                    "env.lblval1",
		"app.kubernetes.io/managed-by": "radius-rp",
		"app.kubernetes.io/name":       "test-route",
		"app.kubernetes.io/part-of":    "test-application",
		"radius.dev/application":       "test-application",
		"radius.dev/resource":          "test-route",
		"radius.dev/resource-type":     "applications.core-httproutes",
	}

	if !envOnly {
		metaAnn["app.ann1"] = "app.annval1"
		metaAnn["app.ann2"] = "app.annval2"
		metaAnn["test.ann1"] = "override.app.annval1"

		metaLbl["app.lbl1"] = "app.lblval1"
		metaLbl["app.lbl2"] = "app.lblval2"
		metaLbl["test.lbl1"] = "override.app.lblval1"

	}

	return &expectedMaps{
		metaAnn: metaAnn,
		metaLbl: metaLbl,
	}
}
