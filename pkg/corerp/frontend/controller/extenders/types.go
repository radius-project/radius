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

package extenders

import (
	"time"
)

const (
	// OperationListSecret is a user defined operation names.
	OperationListSecrets = "LISTSECRETS"

	// ResourceTypeName is the name of the extenders resource type.
	ResourceTypeName = "Applications.Core/extenders"

	// AsyncCreateOrUpdateExtenderTimeout is the timeout for async create or update extender.
	AsyncCreateOrUpdateExtenderTimeout = time.Duration(60) * time.Minute

	// AsyncDeleteExtenderTimeout is the timeout for async delete extender.
	AsyncDeleteExtenderTimeout = time.Duration(30) * time.Minute
)
