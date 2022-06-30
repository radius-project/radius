// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

// GenericEnvironment represents an *unknown* kind of environment.
type GenericEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	DefaultApplication string `mapstructure:"defaultapplication,omitempty"`

	// Capture arbitrary other properties
	Properties map[string]interface{} `mapstructure:",remain"`
}

func (e *GenericEnvironment) GetName() string {
	return e.Name
}

func (e *GenericEnvironment) GetKind() string {
	return e.Kind
}

func (e *GenericEnvironment) GetEnableUCP() bool {
	return false
}

func (e *GenericEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *GenericEnvironment) GetKubeContext() string {
	return ""
}

func (e *GenericEnvironment) GetContainerRegistry() *Registry {
	return nil
}

func (e *GenericEnvironment) GetStatusLink() string {
	return ""
}
