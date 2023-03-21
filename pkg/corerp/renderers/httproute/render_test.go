// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

	// User Inputs for testing labels and annotations
	envAnnotationKey1 = "env.ann1"
	envAnnotationKey2 = "env.ann2"
	envAnnotationVal1 = "env.annval1"
	envAnnotationVal2 = "env.annval2"

	envLabelKey1 = "env.lbl1"
	envLabelKey2 = "env.lbl2"
	envLabelVal1 = "env.lblval1"
	envLabelVal2 = "env.lblval2"

	appAnnotationKey1 = "app.ann1"
	appAnnotationKey2 = "app.ann2"
	appAnnotationVal1 = "app.annval1"
	appAnnotationVal2 = "app.annval2"

	appLabelKey1 = "app.lbl1"
	appLabelKey2 = "app.lbl2"
	appLabelVal1 = "env.lblval1"
	appLabelVal2 = "env.lblval2"

	overrideKey1 = "test.ann1"
	overrideKey2 = "test.lbl1"
	overrideVal1 = "override.app.annval1"
	overrideVal2 = "override.app.lblval1"

	managedbyKey    = "app.kubernetes.io/managed-by"
	managedbyVal    = "radius-rp"
	nameKey         = "app.kubernetes.io/name"
	nameVal         = "test-route"
	partofKey       = "app.kubernetes.io/part-of"
	partofVal       = "test-application"
	appKey          = "radius.dev/application"
	appVal          = "test-application"
	resourceKey     = "radius.dev/resource"
	resourceVal     = "test-route"
	resourcetypeKey = "radius.dev/resource-type"
	resourcetypeVal = "applications.core-httproutes"
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
			name:         "Test_Port",
			port:         6379,
			options:      getRenderOptions(0),
			setupMaps:    nil,
			expectedMaps: nil,
		},
		{
			name:         "Test_DefaultPort",
			port:         renderers.DefaultPort,
			options:      getRenderOptions(0),
			setupMaps:    nil,
			expectedMaps: nil,
		},
		{
			name:         "Test_With_Environment_Kubernetes_Metadata",
			port:         renderers.DefaultPort,
			options:      getRenderOptions(1),
			setupMaps:    getSetUpMaps(true),
			expectedMaps: getExpectedMaps(true),
		},
		{
			name:         "Test_With_Environment_Application_Kubernetes_Metadata",
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
				require.Nil(t, service.Annotations)
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
		opt: 1 - Env KubeMetadata
		opt: 2 - Env and App KubeMetadata
	*/

	dependencies := map[string]renderers.RendererDependency{}
	option := renderers.RenderOptions{Dependencies: dependencies}
	if !(opt == 1 || opt == 2) {
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
			envAnnotationKey1: envAnnotationVal1,
			envAnnotationKey2: envAnnotationVal2,
			overrideKey1:      envAnnotationVal1,
		},
		Labels: map[string]string{
			envLabelKey1: envLabelVal1,
			envLabelKey2: envLabelVal2,
			overrideKey2: envLabelVal1,
		},
	}
	appKubeMetadataExt := &datamodel.KubeMetadataExtension{
		Annotations: map[string]string{
			appAnnotationKey1: appAnnotationVal1,
			appAnnotationKey2: appAnnotationVal2,
			overrideKey1:      overrideVal1,
		},
		Labels: map[string]string{
			appLabelKey1: appLabelVal1,
			appLabelKey2: appLabelVal2,
			overrideKey2: overrideVal2,
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
		envAnnotationKey1: envAnnotationVal1,
		envAnnotationKey2: envAnnotationVal2,
		overrideKey1:      envAnnotationVal1,
	}
	metaLbl := map[string]string{
		envLabelKey1:    envLabelVal1,
		envLabelKey2:    envLabelVal2,
		overrideKey2:    envLabelVal1,
		managedbyKey:    managedbyVal,
		nameKey:         nameVal,
		partofKey:       partofVal,
		appKey:          appVal,
		resourceKey:     resourceVal,
		resourcetypeKey: resourcetypeVal,
	}

	if !envOnly {
		metaAnn[appAnnotationKey1] = appAnnotationVal1
		metaAnn[appAnnotationKey2] = appAnnotationVal2
		metaAnn[overrideKey1] = overrideVal1

		metaLbl[appLabelKey1] = appLabelVal1
		metaLbl[appLabelKey2] = appLabelVal2
		metaLbl[overrideKey2] = overrideVal2
	}

	return &expectedMaps{
		metaAnn: metaAnn,
		metaLbl: metaLbl,
	}
}
