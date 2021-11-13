// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package scaffold

import (
	_ "embed"
)

type ScaffoldTemplate = string

type TemplateOptions struct {
	ApplicationName string
}

//go:embed "app.bicep.tmpl"
var ApplicationStage ScaffoldTemplate

//go:embed "infra.bicep.tmpl"
var InfrastructureStage ScaffoldTemplate

//go:embed "rad.yaml.tmpl"
var RADYaml ScaffoldTemplate
