package kubernetes

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/radrp/k8sauth"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	GatewayServiceName = "haproxy-ingress"
)

type KubernetesClient struct {
	config *rest.Config
	client client.Client
}

func NewKubernetesClient() (*KubernetesClient, error) {
	cfg, err := k8sauth.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	s := scheme.Scheme
	c, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &KubernetesClient{
		config: cfg,
		client: c,
	}, nil
}

func (kc *KubernetesClient) GetPublicIP(ctx context.Context) (*string, error) {
	svc := &corev1.ServiceList{}
	err := kc.client.List(ctx, svc, &client.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, service := range svc.Items {
		if service.Name == GatewayServiceName {
			for _, in := range service.Status.LoadBalancer.Ingress {
				return &in.IP, nil
			}
		}
	}

	return nil, errors.New("no public ip found")
}
