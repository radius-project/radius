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

package testrp

const Version = "2022-03-15-privatepreview"

type TestResourceList struct {
	Value []TestResource `json:"value"`
}

type TestResource struct {
	ID         *string `json:"id"`
	Type       *string `json:"type"`
	Name       *string `json:"name"`
	Location   *string `json:"location"`
	Tags       map[string]*string
	Properties TestResourceProperties `json:"properties,omitempty"`
}

type TestResourceProperties struct {
	Message           *string `json:"message,omitempty"`
	ProvisioningState *string `json:"provisioningState,omitempty"`
}
