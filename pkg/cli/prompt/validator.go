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

package prompt

import (
	"errors"
	"regexp"
)

const (
	invalidNamespaceNameMessage = "namespace must be 1-63 characters, made up of of lower case alphanumeric characters or '-', start with an alphabetic character, and end with an alphanumeric character"
	invalidResourceNameMessage  = "name must be made up of alphanumeric characters and hyphens, and must begin with an alphabetic character and end with an alphanumeric character"
	invalidUUIDv4Message        = "must be a valid UUID v4 (GUID)"

	// ErrExitConsoleMessage is the message that is displayed when the user exits the console. This is exported for use in tests.
	ErrExitConsoleMessage = "exiting command"
)

// ValidateKubernetesNamespace validates the user input according to Kubernetes rules for a namespace name.
//
// Largely matches https://github.com/kubernetes/apimachinery/blob/master/pkg/util/validation/validation.go#LL226C33-L226C59
//
// # Function Explanation
//
// ValidateKubernetesNamespace checks if the input string is a valid Kubernetes namespace name, and returns an error if it is not.
func ValidateKubernetesNamespace(input string) error {
	r := regexp.MustCompile("^[a-z]([-a-z0-9]*[a-z0-9])?$")
	if r.MatchString(input) && len(input) <= 63 {
		return nil
	}

	return errors.New(invalidResourceNameMessage)
}

// ValidateKubernetesNamespaceOrDefault validates the user input according to Kubernetes rules for a namespace name, but also allows empty input.
//
// Largely matches https://github.com/kubernetes/apimachinery/blob/master/pkg/util/validation/validation.go#LL226C33-L226C59
//
// # Function Explanation
//
// ValidateKubernetesNamespaceOrDefault checks if the input is an empty string, and if so, returns nil, otherwise it
// calls ValidateKubernetesNamespace and returns the result. If ValidateKubernetesNamespace returns an error,
// ValidateKubernetesNamespaceOrDefault will also return an error.
func ValidateKubernetesNamespaceOrDefault(input string) error {
	if input == "" {
		return nil
	}

	return ValidateKubernetesNamespace(input)
}

// ValidateResourceName validates the user input according to ARM/UCP rules for a resource name.
//
// Largely matches https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/resource-name-rules
//
// # Function Explanation
//
// ValidateResourceName checks if the given string is a valid resource name, and returns an error if it is not.
func ValidateResourceName(input string) error {
	// Note: resource names vary in length requirements depending on the type, so we don't validate length here.
	r := regexp.MustCompile("^[a-zA-Z]([a-zA-Z0-9-]*[a-zA-Z0-9])?$")
	if r.MatchString(input) {
		return nil
	}

	return errors.New(invalidResourceNameMessage)
}

// ValidateResourceName validates the user input according to ARM/UCP rules for a resource name, but also allows empty input.
//
// Largely matches https://docs.microsoft.com/en-us/azure/azure-resource-manager/management/resource-name-rules
//
// # Function Explanation
//
// ValidateResourceNameOrDefault checks if the input string is empty, and if it is, returns nil, otherwise
// it calls the ValidateResourceName function to check if the input string is valid.
func ValidateResourceNameOrDefault(input string) error {
	if input == "" {
		return nil
	}

	return ValidateResourceName(input)
}

// ValidateUUIDv4 validates the user input according to the rules for a UUID v4 (GUID).
//
// # Function Explanation
//
// ValidateUUIDv4 checks if the input string is a valid UUIDv4 and returns an error if it is not.
func ValidateUUIDv4(input string) error {
	r := regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}$")
	if r.MatchString(input) {
		return nil
	}

	return errors.New(invalidUUIDv4Message)
}
