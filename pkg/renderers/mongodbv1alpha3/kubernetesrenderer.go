// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SecretKeyMongoDBAdminUsername    = "MONGO_ROOT_USERNAME"
	SecretKeyMongoDBAdminPassword    = "MONGO_ROOT_PASSWORD"
	SecretKeyMongoDBConnectionString = "MONGO_CONNECTIONSTRING"
)

var _ renderers.Renderer = (*KubernetesRenderer)(nil)

type KubernetesRenderer struct {
}

type KubernetesOptions struct {
	DescriptiveLabels map[string]string
	SelectorLabels    map[string]string
	Namespace         string
	Name              string
}

func (r *KubernetesRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r *KubernetesRenderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := MongoDBComponentProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if !properties.Managed {
		return renderers.RendererOutput{}, errors.New("only Radius managed resources are supported for MongoDB on Kubernetes")
	}

	options := KubernetesOptions{
		DescriptiveLabels: kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		SelectorLabels:    kubernetes.MakeSelectorLabels(resource.ApplicationName, resource.ResourceName),

		// For now use the component name as the Kubernetes resource name.
		Name: resource.ResourceName,
	}

	resources := []outputresource.OutputResource{}

	// The secret is used to hold the password, just so it's not stored in plaintext.
	//
	// TODO: for now this is VERY hardcoded.
	secret := r.MakeSecret(options, resource.ResourceName, "admin", "password")
	resources = append(resources, outputresource.NewKubernetesOutputResource(outputresource.LocalIDSecret, secret, secret.ObjectMeta))

	// This is a headless service, clients of Mongo will just use it for DNS.
	// Mongo is a replicated service and clients need to know the addresses of the replicas.
	service := r.MakeService(options)
	resources = append(resources, outputresource.NewKubernetesOutputResource(outputresource.LocalIDService, service, service.ObjectMeta))

	set := r.MakeStatefulSet(options, service.Name, secret.Name)
	resources = append(resources, outputresource.NewKubernetesOutputResource(outputresource.LocalIDStatefulSet, set, set.ObjectMeta))

	computedValues := map[string]renderers.ComputedValueReference{
		"database": {
			Value: resource.ResourceName,
		},
	}
	secretValues := map[string]renderers.SecretValueReference{
		"connectionString": {
			LocalID:       outputresource.LocalIDSecret,
			ValueSelector: SecretKeyMongoDBConnectionString,
		},
	}

	return renderers.RendererOutput{
		Resources:      resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func (r KubernetesRenderer) MakeSecret(options KubernetesOptions, service string, username string, password string) *corev1.Secret {
	// Make a connection string and use the secret to store it.

	// For now this is static, the host and database are just the component name.
	port := 27017

	// Mongo connection strings look like: 'mongodb://{accountname}:{key}@{endpoint}:{port}/{logindatabase}?...{params}'
	u := url.URL{
		Scheme: "mongodb",
		User:   url.UserPassword(string(username), string(password)),
		Host:   fmt.Sprintf("%s:%d", service, port),
		Path:   "admin", // is the default for the login database
	}

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: options.Namespace,
			Labels:    options.DescriptiveLabels,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			SecretKeyMongoDBAdminUsername:    []byte(username),
			SecretKeyMongoDBAdminPassword:    []byte(password),
			SecretKeyMongoDBConnectionString: []byte(u.String()),
		},
	}
}

func (r KubernetesRenderer) MakeService(options KubernetesOptions) *corev1.Service {
	// This is a headless service, clients of Mongo will just use it for DNS.
	// Mongo is a replicated service and clients need to know the addresses of the replicas.
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: options.Namespace,
			Labels:    options.DescriptiveLabels,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			Selector:  options.SelectorLabels,
		},
	}
}

func (r KubernetesRenderer) MakeStatefulSet(options KubernetesOptions, service string, secret string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StatefulSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.Name,
			Namespace: options.Namespace,
			Labels:    options.DescriptiveLabels,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: options.SelectorLabels,
			},
			ServiceName: service,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: options.DescriptiveLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "mongo",
							Image: "mongo:5",
							Env: []corev1.EnvVar{
								{
									Name: "MONGO_INITDB_ROOT_USERNAME",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secret,
											},
											Key: SecretKeyMongoDBAdminUsername,
										},
									},
								},
								{
									Name: "MONGO_INITDB_ROOT_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: secret,
											},
											Key: SecretKeyMongoDBAdminPassword,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
