// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package websitev1alpha3

import (
	"encoding/json"
	"fmt"

	"github.com/Azure/radius/pkg/renderers"
)

// Liveness/Readiness constants
const (
	DefaultInitialDelaySeconds = 0
	DefaultFailureThreshold    = 3
	DefaultPeriodSeconds       = 10
	HTTPGet                    = "httpGet"
	TCP                        = "tcp"
	Exec                       = "exec"
)

const (
	kindProperty = "kind"
	ResourceType = "Website"
)

type WebsiteProperties struct {
	Connections    map[string]Connection  `json:"connections,omitempty"`
	Container      *Container             `json:"container,omitempty"`
	Executable     *Executable            `json:"executable,omitempty"`
	Env            map[string]interface{} `json:"env,omitempty"`
	Ports          map[string]WebsitePort `json:"ports,omitempty"`
	ReadinessProbe map[string]interface{} `json:"readinessProbe,omitempty"`
	LivenessProbe  map[string]interface{} `json:"livenessProbe,omitempty"`
	Traits         []Trait                `json:"traits,omitempty"`
}

type Container struct {
	Image string `json:"image"`
}

type Executable struct {
	Name             string   `json:"name"`
	WorkingDirectory string   `json:"workingDirectory,omitempty"`
	Args             []string `json:"args,omitempty"`
}

type WebsitePort struct {
	Provides string `json:"provides"`
	Protocol string `json:"protocol"`
	Port     *int   `json:"port"`
	Dynamic  bool   `json:"dynamic"`
}

type Connection struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
}

// HTTPGetHealthProbe defines the properties when an httpGet readiness/liveness probe is specified
type HTTPGetHealthProbe struct {
	Kind    string            `json:"kind"`
	Path    string            `json:"path"`
	Port    int               `json:"containerPort"`
	Headers map[string]string `json:"headers"`
	// Initial delay in seconds before probing for readiness/liveness
	InitialDelaySeconds *int `json:"initialDelaySeconds"`
	// Threshold number of times the probe fails after which a failure would be reported
	FailureThreshold *int `json:"failureThreshold"`
	// Interval for the readiness/liveness probe in seconds
	PeriodSeconds *int `json:"periodSeconds"`
}

// TCPHealthProbe defines the properties when a tcp readiness/liveness probe is specified
type TCPHealthProbe struct {
	Kind string `json:"kind"`
	Port int    `json:"containerPort"`
	// Initial delay in seconds before probing for readiness/liveness
	InitialDelaySeconds *int `json:"initialDelaySeconds"`
	// Threshold number of times the probe fails after which a failure would be reported
	FailureThreshold *int `json:"failureThreshold"`
	// Interval for the readiness/liveness probe in seconds
	PeriodSeconds *int `json:"periodSeconds"`
}

// ExecHealthProbe defines the properties when an exec readiness/liveness probe is specified
type ExecHealthProbe struct {
	Kind    string `json:"kind"`
	Command string `json:"command"`
	// Initial delay in seconds before probing for readiness/liveness
	InitialDelaySeconds *int `json:"initialDelaySeconds"`
	// Threshold number of times the probe fails after which a failure would be reported
	FailureThreshold *int `json:"failureThreshold"`
	// Interval for the readiness/liveness probe in seconds
	PeriodSeconds *int `json:"periodSeconds"`
}

type Trait struct {
	Kind                 string
	AdditionalProperties map[string]interface{}
}

func (ct Trait) MarshalJSON() ([]byte, error) {
	properties := map[string]interface{}{}
	for k, v := range ct.AdditionalProperties {
		properties[k] = v
	}

	properties[kindProperty] = ct.Kind
	return json.Marshal(properties)
}

func (ct *Trait) UnmarshalJSON(b []byte) error {
	properties := map[string]interface{}{}
	err := json.Unmarshal(b, &properties)
	if err != nil {
		return err
	}

	obj, ok := properties[kindProperty]
	if !ok {
		return fmt.Errorf("the '%s' property is required", kindProperty)
	}

	kind, ok := obj.(string)
	if !ok {
		return fmt.Errorf("the '%s' property must be a string", kindProperty)
	}

	delete(properties, kindProperty)

	ct.Kind = kind
	ct.AdditionalProperties = properties
	return nil
}

func (resource WebsiteProperties) FindTrait(kind string, trait interface{}) (bool, error) {
	traits := resource.Traits
	if traits == nil {
		return false, nil
	}
	for _, v := range traits {
		if v.Kind == kind {
			return v.As(kind, trait)
		}
	}

	return false, nil
}

func (resource Trait) As(kind string, specific interface{}) (bool, error) {
	if resource.Kind != kind {
		return false, nil
	}

	bytes, err := json.Marshal(resource)
	if err != nil {
		return false, fmt.Errorf("failed to marshal generic trait value: %w", err)
	}

	err = json.Unmarshal(bytes, specific)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal JSON as value of type %T: %w", specific, err)
	}

	return true, nil
}

func convert(resource renderers.RendererResource) (*WebsiteProperties, error) {
	properties := &WebsiteProperties{}
	err := resource.ConvertDefinition(properties)
	if err != nil {
		return nil, err
	}

	return properties, nil
}
