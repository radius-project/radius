// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radyaml

import (
	"io"
	"io/ioutil"

	"gopkg.in/yaml.v3"
)

type RADYaml struct {
	Name   string         `yaml:"name"`
	Layers []RADYamlLayer `yaml:"layers,omitempty"`
}

type RADYamlLayer struct {
	Name   string                     `yaml:"name"`
	Build  *[]RADYamlLayerBuildTarget `yaml:"build,omitempty"`
	Deploy *string                    `yaml:"deploy,omitempty"`
}

type RADYamlLayerBuildTarget struct {
	Name   string                  `yaml:"name"`
	Docker *map[string]interface{} `yaml:"docker,omitempty"`
	NPM    *RADYamlNPMBuild        `yaml:"npm,omitempty"`
}

type RADYamlNPMBuild struct {
	WorkingDirectory string            `yaml:"workingDirectory"`
	Args             []string          `yaml:"args,omitempty"`
	Docker           *RADYamlNPMDocker `yaml:"docker,omitempty"`
}

type RADYamlNPMDocker struct {
	Repository string `yaml:"repository"`
}

func Read(reader io.Reader) (RADYaml, error) {
	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return RADYaml{}, nil
	}

	config := RADYaml{}
	err = yaml.Unmarshal(b, &config)
	if err != nil {
		return RADYaml{}, nil
	}

	return config, nil
}
