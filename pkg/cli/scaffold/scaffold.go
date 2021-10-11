// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package scaffold

import (
	"os"
	"path"
	"text/template"
)

func WriteApplication(baseDirectory string) error {
	applicationName := path.Base(baseDirectory)
	options := TemplateOptions{ApplicationName: applicationName}

	appLayer := template.Must(template.New("app.bicep").Parse(ApplicationLayer))
	infraLayer := template.Must(template.New("infra.bicep").Parse(InfrasturctureLayer))
	radYAML := template.Must(template.New("rad.yaml").Parse(RADYaml))

	err := writeTemplate(path.Join(baseDirectory, "app.bicep"), appLayer, options)
	if err != nil {
		return err
	}

	err = writeTemplate(path.Join(baseDirectory, "infra.bicep"), infraLayer, options)
	if err != nil {
		return err
	}

	err = writeTemplate(path.Join(baseDirectory, "rad.yaml"), radYAML, options)
	if err != nil {
		return err
	}

	return nil
}

func writeTemplate(filePath string, template *template.Template, options TemplateOptions) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	err = template.Execute(file, options)
	if err != nil {
		return err
	}

	return nil
}
