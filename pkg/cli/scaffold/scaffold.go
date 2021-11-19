// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package scaffold

import (
	"fmt"
	"os"
	"path"
	"text/template"

	"github.com/Azure/radius/pkg/cli"
)

type Options struct {
	ApplicationName string
	BaseDirectory   string
	Force           bool
}

// WriteApplication scaffolds a new Radius application and returns the set of files created.
func WriteApplication(options Options) ([]string, error) {
	templates := GetApplicationTemplates()

	if !options.Force {
		// Check up front if any of the files we would create already exist
		for _, template := range templates {
			filePath := path.Join(options.BaseDirectory, template.FilePath)
			_, err := os.Stat(path.Join(options.BaseDirectory, template.FilePath))
			if os.IsNotExist(err) {
				continue // Cool, keep going
			} else if err != nil {
				return nil, err // Failed to stat == likely permissions erro
			} else {
				message := fmt.Sprintf("The file %q already exists. Specify --force to overwrite.", filePath)
				return nil, &cli.FriendlyError{Message: message} // File already exists, bail.
			}
		}
	}

	err := os.MkdirAll(path.Join(options.BaseDirectory, "rad"), 0755)
	if err != nil {
		return nil, err
	}

	outputs := []string{}
	for _, template := range templates {
		filePath := path.Join(options.BaseDirectory, template.FilePath)
		err = writeTemplate(filePath, template.Template, TemplateOptions{ApplicationName: options.ApplicationName})
		if err != nil {
			return nil, err
		}

		// NOTE: we're intentionally using the relative path here for simpler display.
		outputs = append(outputs, template.FilePath)
	}

	return outputs, nil
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
