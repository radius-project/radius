// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package providers

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestGetDeployedResource(t *testing.T) {
	for _, tc := range []struct {
		name        string
		resources   []runtime.Object
		ref         string
		version     string
		expectedErr string
		expected    map[string]interface{}
	}{{
		name:        "not found",
		ref:         "kubernetes.apps/Deployment/not-exist",
		version:     "v1",
		expectedErr: `deployments.apps "not-exist" not found`,
	}, {
		name:        "wrong kind",
		ref:         "kubernetes.apps/Service/corev1-not-appsv1",
		version:     "v1",
		expectedErr: "no matches for kind",
	}, {
		name: "service",
		resources: []runtime.Object{
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "redis-master",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
		},
		ref:     "kubernetes.core/Service/redis-master",
		version: "v1",
		expected: map[string]interface{}{
			"properties": map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]interface{}{
					"name":      "redis-master",
					"namespace": "default",
					// This was defaulted, but wasn't omitted in their marshaller.
					"creationTimestamp": nil,
				},
				"spec": map[string]interface{}{
					"type": "ClusterIP",
				},
				// This was defaulted, but wasn't omitted in their marshaller.
				"status": map[string]interface{}{
					"loadBalancer": map[string]interface{}{},
				},
			},
		},
	}, {
		name: "secret",
		resources: []runtime.Object{
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "redis",
					Namespace: "default",
				},
				StringData: map[string]string{
					"redis-password": "unsecure",
				},
			},
		},
		ref:     "kubernetes.core/Secret/redis",
		version: "v1",
		expected: map[string]interface{}{
			"properties": map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "redis",
					"namespace": "default",
					// This was defaulted, but wasn't omitted in their marshaller.
					"creationTimestamp": nil,
				},
				"stringData": map[string]interface{}{
					"redis-password": "unsecure",
				},
			},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			fakeDynamicClient := fake.NewSimpleDynamicClient(fakeScheme(), tc.resources...)
			store := NewK8sProvider(
				logr.FromContext(context.Background()),
				fakeDynamicClient,
				fakeRestMapper(),
			)
			output, err := store.GetDeployedResource(context.Background(), tc.ref, tc.version)
			if tc.expectedErr == "" {
				require.NoError(t, err)
			}
			if err != nil {
				require.Regexp(t, tc.expectedErr, err.Error())
				return
			}
			assert.DeepEqual(t, tc.expected, output)
		})
	}
}

func TestExtractGroupVersionResourceName(t *testing.T) {
	store := NewK8sProvider(
		logr.FromContext(context.Background()),
		fake.NewSimpleDynamicClient(runtime.NewScheme()),
		fakeRestMapper(),
	)

	for _, tc := range []struct {
		name         string
		ref          string
		version      string
		expectedGvr  schema.GroupVersionResource
		expectedName string
		expectedErr  string
	}{{
		name:        "wrong type",
		ref:         "this/looks/like/arm/Resource/name",
		expectedErr: "wrong reference format",
	}, {
		name:    "corev1/Secret",
		ref:     "grandparent/parent/kubernetes.core/Secret/mine",
		version: "v1",
		expectedGvr: schema.GroupVersionResource{
			Version:  "v1",
			Resource: "secrets",
		},
		expectedName: "mine",
	}, {
		name:    "appsv1/Deployment",
		ref:     "grandparent/parent/kubernetes.apps/Deployment/deploymentName",
		version: "v1",
		expectedGvr: schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		expectedName: "deploymentName",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			gvr, _, name, err := store.extractGroupVersionResourceName(tc.ref, tc.version)
			if tc.expectedErr == "" {
				require.NoError(t, err)
			}
			if err != nil {
				require.Regexp(t, tc.expectedErr, err.Error())
				return
			}
			assert.Equal(t, tc.expectedName, name)
			assert.Equal(t, tc.expectedGvr, gvr)
		})
	}
}

func fakeScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	return scheme
}

func fakeRestMapper() meta.RESTMapper {
	restMapper := meta.NewDefaultRESTMapper(nil)
	restMapper.AddSpecific(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "Secret",
	}, schema.GroupVersionResource{
		Version:  "v1",
		Resource: "secrets",
	}, schema.GroupVersionResource{
		Version:  "v1",
		Resource: "secrets",
	}, meta.RESTScopeNamespace)
	restMapper.AddSpecific(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}, schema.GroupVersionResource{
		Group:    "apps",
		Version:  "v1",
		Resource: "deployments",
	}, meta.RESTScopeNamespace)
	restMapper.AddSpecific(schema.GroupVersionKind{
		Version: "v1",
		Kind:    "Service",
	}, schema.GroupVersionResource{
		Version:  "v1",
		Resource: "services",
	}, schema.GroupVersionResource{
		Version:  "v1",
		Resource: "services",
	}, meta.RESTScopeNamespace)
	return restMapper
}
