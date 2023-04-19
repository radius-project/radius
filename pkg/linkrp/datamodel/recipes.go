// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// This is the version nuetral data model. Structures present in this package must be used internally.
// API version specific models should be exist under /pkg/linkrp/api. This api package should be able
// to convert among the versions.

package datamodel

import (
	"github.com/project-radius/radius/pkg/linkrp"
)

// RecipeDataModel should be implemented on the datamodel of types that support recipes.
type RecipeDataModel interface {
	// Recipe provides access to the user-specified recipe configuration. Can return nil.
	Recipe() *linkrp.LinkRecipe
}
