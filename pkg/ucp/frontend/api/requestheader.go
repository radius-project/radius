// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package api

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func loadRequestHeaderConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("create in-cluster request-header client configuration: %w", err)
	}
	return requestHeaderConfigMap(ctx, config)
}

func requestHeaderConfigMap(ctx context.Context, config *rest.Config) (*corev1.ConfigMap, error) {
	config = rest.CopyConfig(config)
	config.Timeout = 10 * time.Second
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create request-header Kubernetes client: %w", err)
	}
	configMap, err := client.CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(ctx, "extension-apiserver-authentication", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get kube-system/extension-apiserver-authentication: %w", err)
	}
	return configMap, nil
}
