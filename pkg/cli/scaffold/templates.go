// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package scaffold

import (
	_ "embed"
	"path"
	"text/template"
)

type TemplateWorkItem struct {
	FilePath string
	Template *template.Template
}

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

func GetApplicationTemplates() []TemplateWorkItem {
	return []TemplateWorkItem{
		{
			FilePath: path.Join("rad.yaml"),
			Template: template.Must(template.New("rad.yaml").Parse(RADYaml)),
		},
		{
			FilePath: path.Join("iac", "infra.bicep"),
			Template: template.Must(template.New("infra.bicep").Parse(InfrastructureStage)),
		},
		{
			FilePath: path.Join("iac", "app.bicep"),
			Template: template.Must(template.New("app.bicep").Parse(ApplicationStage)),
		},
	}
}
