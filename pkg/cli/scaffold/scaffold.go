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

// WriteApplication scaffolds a new Radius application and returns the set of files created.
func WriteApplication(baseDirectory string, applicationName string) ([]string, error) {
	options := TemplateOptions{ApplicationName: applicationName}

	err := os.MkdirAll(path.Join(baseDirectory, "rad"), 0755)
	if err != nil {
		return nil, err
	}

	radYAML := template.Must(template.New("rad.yaml").Parse(RADYaml))
	err = writeTemplate(path.Join(baseDirectory, "rad", "rad.yaml"), radYAML, options)
	if err != nil {
		return nil, err
	}

	infraLayer := template.Must(template.New("infra.bicep").Parse(InfrastructureStage))
	err = writeTemplate(path.Join(baseDirectory, "rad", "infra.bicep"), infraLayer, options)
	if err != nil {
		return nil, err
	}

	appLayer := template.Must(template.New("app.bicep").Parse(ApplicationStage))
	err = writeTemplate(path.Join(baseDirectory, "rad", "app.bicep"), appLayer, options)
	if err != nil {
		return nil, err
	}

	return []string{
		path.Join("rad", "rad.yaml"),
		path.Join("rad", "infra.bicep"),
		path.Join("rad", "app.bicep"),
	}, nil
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
