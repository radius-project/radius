// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

// GenericEnvironment represents an *unknown* kind of environment.
type GenericEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	Purpose            string `mapstructure:"purpose" yaml:",omitempty"`
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

func (e *GenericEnvironment) GetPurpose() string {
	return e.Purpose
}

func (e *GenericEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

func (e *GenericEnvironment) GetStatusLink() string {
	return ""
}
