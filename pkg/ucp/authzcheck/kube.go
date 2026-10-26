/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authzcheck

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// AuthenticationConfigMapNamespace and AuthenticationConfigMapName locate the
	// cluster's request header (front proxy) settings.
	AuthenticationConfigMapNamespace = "kube-system"
	AuthenticationConfigMapName      = "extension-apiserver-authentication"

	refreshInterval = 5 * time.Minute
)

// FrontProxyConfig is the request header CA and allowed client names.
type FrontProxyConfig struct {
	Roots        *x509.CertPool
	AllowedNames []string
}

// Verify checks that certs chain to the request header CA, are valid for client
// authentication, and, when allowed names are set, that the leaf's common name is one of them.
func (c *FrontProxyConfig) Verify(certs []*x509.Certificate) error {
	if len(certs) == 0 {
		return errors.New("no client certificate")
	}
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}
	_, err := certs[0].Verify(x509.VerifyOptions{
		Roots:         c.Roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return fmt.Errorf("client certificate is not signed by the request header CA: %w", err)
	}
	if len(c.AllowedNames) > 0 && !slices.Contains(c.AllowedNames, certs[0].Subject.CommonName) {
		return fmt.Errorf("client certificate common name %q is not an allowed request header name", certs[0].Subject.CommonName)
	}
	return nil
}

// ParseFrontProxyConfig reads the request header CA and allowed names from the
// extension-apiserver-authentication ConfigMap data.
func ParseFrontProxyConfig(data map[string]string) (*FrontProxyConfig, error) {
	caPEM := data["requestheader-client-ca-file"]
	if caPEM == "" {
		return nil, errors.New("requestheader-client-ca-file is not set")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(caPEM)) {
		return nil, errors.New("requestheader-client-ca-file has no certificates")
	}
	config := &FrontProxyConfig{Roots: roots}
	if names := data["requestheader-allowed-names"]; names != "" {
		if err := json.Unmarshal([]byte(names), &config.AllowedNames); err != nil {
			return nil, fmt.Errorf("failed to parse requestheader-allowed-names: %w", err)
		}
	}
	return config, nil
}

// KubeFrontProxyVerifier loads FrontProxyConfig from the cluster and refreshes it.
type KubeFrontProxyVerifier struct {
	Client kubernetes.Interface

	mu      sync.Mutex
	config  *FrontProxyConfig
	fetched time.Time
}

// Verify implements FrontProxyVerifier.
func (v *KubeFrontProxyVerifier) Verify(ctx context.Context, certs []*x509.Certificate) error {
	config, err := v.load(ctx)
	if err != nil {
		return err
	}
	return config.Verify(certs)
}

func (v *KubeFrontProxyVerifier) load(ctx context.Context) (*FrontProxyConfig, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.config != nil && time.Since(v.fetched) < refreshInterval {
		return v.config, nil
	}
	cm, err := v.Client.CoreV1().ConfigMaps(AuthenticationConfigMapNamespace).Get(ctx, AuthenticationConfigMapName, metav1.GetOptions{})
	if err != nil {
		if v.config != nil {
			return v.config, nil
		}
		return nil, fmt.Errorf("failed to read %s/%s: %w", AuthenticationConfigMapNamespace, AuthenticationConfigMapName, err)
	}
	config, err := ParseFrontProxyConfig(cm.Data)
	if err != nil {
		return nil, err
	}
	v.config = config
	v.fetched = time.Now()
	return config, nil
}

// KubeEngineResolver matches IPs against running deployment engine pods.
type KubeEngineResolver struct {
	Client    kubernetes.Interface
	Namespace string

	mu      sync.Mutex
	ips     map[string]bool
	fetched time.Time
}

// IsEngine implements EngineResolver.
func (e *KubeEngineResolver) IsEngine(ctx context.Context, ip string) bool {
	if ip == "" {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ips != nil && e.ips[ip] {
		return true
	}
	// Refresh on a miss, at most every 5 seconds, so a restarted engine pod is found quickly.
	if time.Since(e.fetched) < 5*time.Second {
		return false
	}
	pods, err := e.Client.CoreV1().Pods(e.Namespace).List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + ComponentDeploymentEngine})
	e.fetched = time.Now()
	if err != nil {
		return false
	}
	e.ips = map[string]bool{}
	for _, pod := range pods.Items {
		for _, podIP := range pod.Status.PodIPs {
			e.ips[podIP.IP] = true
		}
	}
	return e.ips[ip]
}
