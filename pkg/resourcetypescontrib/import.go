// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package resourcetypescontrib exists solely to keep the
// github.com/radius-project/resource-types-contrib dependency in go.mod.
//
// resource-types-contrib is a Go module containing YAML manifest files (no
// executable Go code). The blank import below prevents "go mod tidy" from
// removing the dependency. The actual manifest files are copied from the
// module cache into deploy/manifest/built-in-providers/ by
// "make sync-resource-types".
package resourcetypescontrib

import (
	// Blank import to retain the resource-types-contrib module dependency in
	// go.mod. Without this import, "go mod tidy" would remove the dependency
	// because no Go code directly references the module.
	_ "github.com/radius-project/resource-types-contrib"
)
