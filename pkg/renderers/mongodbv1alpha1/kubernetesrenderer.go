// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha1

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	SecretKeyMongoDBAdminUsername = "MONGO_ROOT_USERNAME"
	SecretKeyMongoDBAdminPassword = "MONGO_ROOT_PASSWORD"
)

type KubernetesRenderer struct {
	K8s client.Client
}

type KubernetesOptions struct {
	DescriptiveLabels map[string]string
	SelectorLabels    map[string]string
	Namespace         string
	Name              string
}

var _ workloads.WorkloadRenderer = (*KubernetesRenderer)(nil)

func (r KubernetesRenderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	// TODO: right now we need to hardcode this because we can't easily take a dependency on the secret
	// inside Kubernetes. AllocateBindings does not get access to the secret's state, and the secret
	// has not been created the first time we render.
	username := "admin"
	password := "password"

	// For now this is static, the host and database are just the component name.
	service := workload.Name
	database := workload.Name
	port := 27017

	namespace := workload.Namespace
	if namespace == "" {
		namespace = workload.Application
	}

	// Mongo connection strings look like: 'mongodb://{accountname}:{key}@{endpoint}:{port}/{logindatabase}?...{params}'
	u := url.URL{
		Scheme: "mongodb",
		User:   url.UserPassword(string(username), string(password)),
		Host:   fmt.Sprintf("%s.%s.svc.cluster.local:%d", service, namespace, port),
		Path:   "admin", // is the default for the login database
	}

	bindings := map[string]components.BindingState{
		BindingMongo: {
			Component: workload.Name,
			Binding:   BindingMongo,
			Kind:      "mongodb.com/Mongo",
			Properties: map[string]interface{}{
				"connectionString": u.String(),
				"database":         database,
			},
		},
	}

	return bindings, nil
}

func (r KubernetesRenderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := MongoDBComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	if !component.Config.Managed {
		return nil, errors.New("only Radius managed resources are supported for MongoDB on Kubernetes")
	}

	options := KubernetesOptions{
		DescriptiveLabels: kubernetes.MakeDescriptiveLabels(w.Application, w.Name),
		SelectorLabels:    kubernetes.MakeSelectorLabels(w.Application, w.Name),

		// For now use the component name as the Kubernetes resource name.
		Name:      w.Name,
		Namespace: w.Namespace,
	}

	if options.Namespace == "" {
		options.Namespace = w.Application
	}

	resources := []outputresource.OutputResource{}

	// The secret is used to hold the password, just so it's not stored in plaintext.
	//
	// TODO: for now this is VERY hardcoded.
	secret := r.MakeSecret(options, "admin", "password")
	resources = append(resources, outputresource.OutputResource{
		Resource: secret,
		Kind:     outputresource.KindKubernetes,
		LocalID:  outputresource.LocalIDSecret,
		Managed:  true,
		Type:     outputresource.TypeKubernetes,
		Info: outputresource.K8sInfo{
			Kind:       secret.TypeMeta.Kind,
			APIVersion: secret.TypeMeta.APIVersion,
			Name:       secret.ObjectMeta.Name,
			Namespace:  secret.ObjectMeta.Namespace,
		},
	})

	// This is a headless service, clients of Mongo will just use it for DNS.
	// Mongo is a replicated service and clients need to know the addresses of the replicas.
	service := r.MakeService(options)
	resources = append(resources, outputresource.OutputResource{
		Resource: service,
		Kind:     outputresource.KindKubernetes,
		LocalID:  outputresource.LocalIDService,
		Managed:  true,
		Type:     outputresource.TypeKubernetes,
		Info: outputresource.K8sInfo{
			Kind:       service.TypeMeta.Kind,
			APIVersion: service.TypeMeta.APIVersion,
			Name:       service.ObjectMeta.Name,
			Namespace:  service.ObjectMeta.Namespace,
		},
	})

	set := r.MakeStatefulSet(options, service.Name, secret.Name)
	resources = append(resources, outputresource.OutputResource{
		Resource: set,
		Kind:     outputresource.KindKubernetes,
		LocalID:  outputresource.LocalIDStatefulSet,
		Managed:  true,
		Type:     outputresource.TypeKubernetes,
		Info: outputresource.K8sInfo{
			Kind:       set.TypeMeta.Kind,
			APIVersion: set.TypeMeta.APIVersion,
			Name:       set.ObjectMeta.Name,
			Namespace:  set.ObjectMeta.Namespace,
		},
	})

	return resources, nil
}

func (r KubernetesRenderer) MakeSecret(options KubernetesOptions, username string, password string) *corev1.Secret {
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
			SecretKeyMongoDBAdminUsername: []byte(username),
			SecretKeyMongoDBAdminPassword: []byte(password),
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
