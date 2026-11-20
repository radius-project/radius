/*
Copyright 2026 The Radius Authors.

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

package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	radapp "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	inputparameters "github.com/radius-project/radius/pkg/controller/parameters"
	"github.com/radius-project/radius/pkg/hashutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const secretReaderAccount = "radius-deployer"

// SecretInputResolver performs fresh authorized reads at the deployment boundary.
type SecretInputResolver struct {
	accountReader client.Reader
	newReader     func(namespace string) (client.Reader, error)
}

// NewSecretInputResolver uses uncached clients. Only the reader-account GET uses
// the controller identity; Secret GETs always use the fixed namespace identity.
func NewSecretInputResolver(config *rest.Config) (*SecretInputResolver, error) {
	accountReader, err := client.New(config, client.Options{})
	if err != nil {
		return nil, err
	}
	return &SecretInputResolver{
		accountReader: accountReader,
		newReader: func(namespace string) (client.Reader, error) {
			impersonated := rest.CopyConfig(config)
			impersonated.Impersonate = rest.ImpersonationConfig{
				UserName: "system:serviceaccount:" + namespace + ":" + secretReaderAccount,
			}
			return client.New(impersonated, client.Options{})
		},
	}, nil
}

type secretRevision struct {
	Name            string `json:"name"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resourceVersion"`
}

// Resolve returns in-memory ARM inputs and a revision derived only from spec and
// Secret identity/version metadata. No read failures include underlying API errors.
func (r *SecretInputResolver) Resolve(ctx context.Context, deployment *radapp.DeploymentTemplate) (map[string]map[string]string, string, error) {
	if err := inputparameters.Validate(deployment.Spec.Template, deployment.Spec.Parameters, deployment.Spec.ParameterReferences); err != nil {
		return nil, "", err
	}
	if r == nil {
		return nil, "", fmt.Errorf("SecretInputUnavailable: the authorized input reader is not configured")
	}
	account := &corev1.ServiceAccount{}
	key := client.ObjectKey{Namespace: deployment.Namespace, Name: secretReaderAccount}
	if err := r.accountReader.Get(ctx, key, account); err != nil {
		return nil, "", fmt.Errorf("SecretInputUnavailable: provision radius-deployer and its namespace permissions")
	}
	reader, err := r.newReader(deployment.Namespace)
	if err != nil {
		return nil, "", fmt.Errorf("SecretInputUnavailable: configure namespace reader impersonation")
	}

	names := make([]string, 0, len(deployment.Spec.ParameterReferences))
	for name := range deployment.Spec.ParameterReferences {
		names = append(names, name)
	}
	sort.Strings(names)
	parameters := convertToARMJSONParameters(deployment.Spec.Parameters)
	secrets := map[string]*corev1.Secret{}
	revisions := []secretRevision{}
	totalBytes := 0
	for _, name := range names {
		reference := deployment.Spec.ParameterReferences[name].SecretKeyRef
		secret, ok := secrets[reference.Name]
		if !ok {
			secret = &corev1.Secret{}
			if err := reader.Get(ctx, client.ObjectKey{Namespace: deployment.Namespace, Name: reference.Name}, secret); err != nil {
				return nil, "", fmt.Errorf("SecretInputUnavailable: parameter %q cannot be read; check namespace reader permissions and provisioning", name)
			}
			secrets[reference.Name] = secret
			revisions = append(revisions, secretRevision{
				Name: reference.Name, UID: string(secret.UID), ResourceVersion: secret.ResourceVersion,
			})
		}
		data, exists := secret.Data[reference.Key]
		if !exists {
			return nil, "", fmt.Errorf("SecretInputUnavailable: parameter %q has no readable key; check input provisioning", name)
		}
		value := string(data)
		if err := inputparameters.ValidateValue(deployment.Spec.Template, name, value); err != nil {
			return nil, "", err
		}
		totalBytes += len(data)
		if totalBytes > 256*1024 {
			return nil, "", fmt.Errorf("SecretInputInvalid: resolved inputs exceed the 256 KiB total limit")
		}
		parameters[name] = map[string]string{"value": value}
	}
	specHash, err := computeHash(deployment)
	if err != nil {
		return nil, "", err
	}
	revision, err := json.Marshal(struct {
		Spec      string           `json:"spec"`
		Namespace string           `json:"namespace"`
		Secrets   []secretRevision `json:"secrets"`
	}{specHash, deployment.Namespace, revisions})
	if err != nil {
		return nil, "", err
	}
	return parameters, hashutil.Hex(revision), nil
}

func safeDeploymentError(deployment *radapp.DeploymentTemplate, err error) error {
	if err != nil && (len(deployment.Spec.ParameterReferences) != 0 || deployment.Status.AttemptedInputRevision != "") {
		return fmt.Errorf("deployment operation failed; inspect restricted engine diagnostics and retry")
	}
	return err
}
