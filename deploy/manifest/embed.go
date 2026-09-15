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

// Package manifest embeds the product-shipped asset files that live in this
// directory so Go code can read them from any binary. It deliberately holds no
// behavior: everything that parses or interprets these bytes lives in
// [github.com/radius-project/radius/pkg/defaults].
//
// The split exists because the two halves have different owners. The assets are
// deployment artifacts — build/scripts/sync-resource-types.sh rewrites them,
// build/docker.mk copies built-in-providers/ into the ucpd image, and
// build/generate.mk reads defaults.yaml — so they belong under deploy/. A
// //go:embed pattern cannot name a path outside its own package directory, so
// the embed directive has to sit beside the assets even though the code that
// uses it does not.
package manifest

import "embed"

// FS holds the product-shipped assets. Reads return a fresh copy, so callers
// cannot corrupt the bytes another consumer will see.
//
//go:embed default-icon.svg defaults.yaml built-in-providers/self-hosted/*.svg
var FS embed.FS

const (
	// DefaultIconPath is the [FS] path of the product default resource-type icon.
	DefaultIconPath = "default-icon.svg"

	// DefaultsYAMLPath is the [FS] path of the catalog declaring which resource
	// types ship as defaults and which upstream revisions they are pinned to.
	DefaultsYAMLPath = "defaults.yaml"

	// BuiltInIconsDir is the [FS] directory holding the per-type icons mirrored
	// from resource-types-contrib. A type's icon is at
	// BuiltInIconsDir + "/<typeName>.svg".
	BuiltInIconsDir = "built-in-providers/self-hosted"
)
