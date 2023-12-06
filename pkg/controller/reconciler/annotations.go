/*
Copyright 2023.

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
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
)

type deploymentPhrase string

const (
	deploymentPhraseWaiting  deploymentPhrase = "Waiting"
	deploymentPhraseUpdating deploymentPhrase = "Updating"
	deploymentPhraseReady    deploymentPhrase = "Ready"
	deploymentPhraseDeleting deploymentPhrase = "Deleting"
	deploymentPhraseFailed   deploymentPhrase = "Failed"
)

// deploymentAnnotations represents the user-provided configuration and the status (Radius related status)
// of the Deployment.
type deploymentAnnotations struct {
	// Configuration is the configuration of the Deployment provided by the user via annotations.
	// This will be nil if Radius is not enabled for the Deployment.
	Configuration *deploymentConfiguration

	// ConfigurationHash is the hash of the user-provided configuration.
	// This will be used to diff the configuration and determine if the Deployment needs to be updated.
	ConfigurationHash string

	// Status is the status of the Deployment (Radius related status).
	Status *deploymentStatus
}

// There are 4 cases that is possible based on the previous state and the current state of the Deployment:
// Case 1: Previous State: Enabled - Current State: Disabled
// Case 2: Previous State: Disabled - Current State: Enabled
// Case 3: Previous State: Enabled - Current State: Enabled
// Case 4: Previous State: Disabled - Current State: Disabled
//
// How to understand the previous state:
// 1. If "radapp.io/status" annotation is set, then the previous state is Enabled.
//
// Ways to disable Radius:
// 1. "radapp.io/enabled" annotation is set to "false".
// 2. "radapp.io/enabled" annotation is not set.
//
// Ways to enable Radius:
// 1. "radapp.io/enabled" annotation is set to "true".

// deploymentConfiguration is the configuration of the Deployment provided by the user via annotations.
type deploymentConfiguration struct {
	Application string            `json:"application,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Connections map[string]string `json:"connections,omitempty"`
}

func (c *deploymentConfiguration) computeHash() (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	sum := sha1.Sum(b)
	hash := hex.EncodeToString(sum[:])
	return hash, nil
}

type deploymentStatus struct {
	Scope       string                              `json:"scope,omitempty"`
	Application string                              `json:"application,omitempty"`
	Environment string                              `json:"environment,omitempty"`
	Container   string                              `json:"container,omitempty"`
	Operation   *radappiov1alpha3.ResourceOperation `json:"operation,omitempty"`
	Phrase      deploymentPhrase                    `json:"phrase,omitempty"`
}

// readAnnotations reads the annotations from a Deployment.
//
// This includes the configuration specified by the user, the hash of the configuration, and the status.
func readAnnotations(deployment *appsv1.Deployment) (deploymentAnnotations, error) {
	if deployment.Annotations == nil {
		return deploymentAnnotations{}, nil
	}

	result := deploymentAnnotations{
		ConfigurationHash: deployment.Annotations[AnnotationRadiusConfigurationHash],
	}

	s := deploymentStatus{}
	status := deployment.Annotations[AnnotationRadiusStatus]
	if status != "" {
		err := json.Unmarshal([]byte(status), &s)
		if err != nil {
			return result, fmt.Errorf("failed to unmarshal status annotation: %w", err)
		}

		result.Status = &s
	}

	// Note: we need to read and return the configuration even if Radius is not enabled for the Deployment.
	// This is important so that can clean up previously created connections when Radius is disabled.
	enabled := deployment.Annotations[AnnotationRadiusEnabled]
	if !strings.EqualFold(enabled, "true") {
		return result, nil
	}

	result.Configuration = &deploymentConfiguration{
		Environment: deployment.Annotations[AnnotationRadiusEnvironment],
		Application: deployment.Annotations[AnnotationRadiusApplication],
		Connections: map[string]string{},
	}

	for k, v := range deployment.Annotations {
		if strings.HasPrefix(k, AnnotationRadiusConnectionPrefix) {
			result.Configuration.Connections[strings.TrimPrefix(k, AnnotationRadiusConnectionPrefix)] = v
		}
	}

	return result, nil
}

// ApplyToDeployment applies the configuration and status to a Deployment.
//
// This should be used before saving the Deployment's state.
func (annotations *deploymentAnnotations) ApplyToDeployment(deployment *appsv1.Deployment) error {
	if deployment.Annotations == nil {
		deployment.Annotations = map[string]string{}
	}

	status := ""
	if annotations.Status != nil {
		b, err := json.Marshal(annotations.Status)
		if err != nil {
			return err
		}

		status = string(b)
	}

	deployment.Annotations[AnnotationRadiusStatus] = status

	if annotations.Configuration == nil {
		deployment.Annotations[AnnotationRadiusEnabled] = "false"
		return nil
	}

	hash, err := annotations.Configuration.computeHash()
	if err != nil {
		return err
	}

	deployment.Annotations[AnnotationRadiusConfigurationHash] = hash
	deployment.Annotations[AnnotationRadiusEnabled] = "true"

	for k, v := range annotations.Configuration.Connections {
		deployment.Annotations[AnnotationRadiusConnectionPrefix+k] = v
	}

	return nil
}

// IsUpToDate returns true if the Deployment is up to date with the configuration.
//
// This should be used to determine if the Radius container needs to be updated based
// on a change made by the user.
func (annotations *deploymentAnnotations) IsUpToDate() bool {
	if annotations.ConfigurationHash == "" {
		return false
	}

	if annotations.Status == nil {
		return false
	}

	hash, err := annotations.Configuration.computeHash()
	if err != nil {
		return false // If the hash cannot be computed, we assume the configuration is outdated.
	}

	return hash == annotations.ConfigurationHash
}

// OperationInProgress returns true if there is an operation in progress for the given deployment.
func (annotations *deploymentAnnotations) OperationInProgress() bool {
	return annotations.Status != nil && annotations.Status.Operation != nil
}

// isRadiusEnabled returns true if Radius is enabled for the given deployment.
func (annotations *deploymentAnnotations) isRadiusEnabled() bool {
	return annotations.Configuration != nil
}

// needsCleanup returns true if Radius was previously enabled on the deployment and now is disabled.
// This means that we need to clean up the resources created by Radius.
func (annotations *deploymentAnnotations) needsCleanup() bool {
	return annotations.Configuration == nil && annotations.Status != nil
}
