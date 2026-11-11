// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// KubernetesRequestHeaderOptions selects the cluster's published aggregation
// trust instead of a separately configured CA file and proxy names.
type KubernetesRequestHeaderOptions struct {
	// KubernetesIssuer is the stable kubernetes:<trust-domain> identity issuer.
	KubernetesIssuer string `yaml:"kubernetesIssuer"`
}

// NewKubernetesRequestHeaderAuthenticator snapshots the dedicated proxy CA and
// allowed names from kube-system/extension-apiserver-authentication. It never
// uses the general client-ca-file, which also trusts ordinary Kubernetes users.
func NewKubernetesRequestHeaderAuthenticator(configMap *corev1.ConfigMap, options KubernetesRequestHeaderOptions) (*RequestHeaderAuthenticator, error) {
	if configMap == nil {
		return nil, fmt.Errorf("Kubernetes request-header ConfigMap is missing")
	}
	for _, field := range []struct {
		key   string
		value string
	}{
		{"requestheader-username-headers", "X-Remote-User"},
		{"requestheader-group-headers", "X-Remote-Group"},
		{"requestheader-extra-headers-prefix", "X-Remote-Extra-"},
	} {
		headers, err := requestHeaderNames(configMap.Data, field.key)
		if err != nil {
			return nil, err
		}
		if len(headers) != 1 || !strings.EqualFold(headers[0], field.value) {
			return nil, fmt.Errorf("Kubernetes %s must contain only %q", field.key, field.value)
		}
	}
	if _, configured := configMap.Data["requestheader-uid-headers"]; configured {
		headers, err := requestHeaderNames(configMap.Data, "requestheader-uid-headers")
		if err != nil {
			return nil, err
		}
		if len(headers) > 1 || len(headers) == 1 && !strings.EqualFold(headers[0], "X-Remote-Uid") {
			return nil, fmt.Errorf("Kubernetes requestheader-uid-headers must be empty or contain only %q", "X-Remote-Uid")
		}
	}
	names, err := requestHeaderNames(configMap.Data, "requestheader-allowed-names")
	if err != nil {
		return nil, err
	}
	return newRequestHeaderAuthenticator(&RequestHeaderOptions{
		AllowedNames: names, KubernetesIssuer: options.KubernetesIssuer,
	}, func() ([]byte, error) {
		return []byte(configMap.Data["requestheader-client-ca-file"]), nil
	})
}

func requestHeaderNames(data map[string]string, key string) ([]string, error) {
	var names []string
	if err := json.Unmarshal([]byte(data[key]), &names); err != nil {
		return nil, fmt.Errorf("parse Kubernetes %s: %w", key, err)
	}
	if names == nil {
		return nil, fmt.Errorf("Kubernetes %s must be a JSON array, not null", key)
	}
	return names, nil
}
