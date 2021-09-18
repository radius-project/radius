// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package schema

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"
	"unicode"
)

var (
	ResourceBasePath    = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/{applicationName}"
	ApplicationBasePath = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3"
	ApplicationType     = "Application"

	//go:embed resource_boilerplate.json
	boilerplateTemplateText string
	boilerplateTemplate     = template.Must(template.New("resourceBoilerplateTemplate").Parse(boilerplateTemplateText))
)

type resourceInfo struct {
	QualifiedName     string
	ResourcePath      string
	ListType          string
	ResourceType      string
	NameParameterType string
	NameParameterName string
}

func newResourceInfo(qualifiedName, resourcePath string) resourceInfo {
	tokens := strings.Split(resourcePath, "/")
	resourceType := tokens[len(tokens)-1]
	base := strings.TrimSuffix(resourceType, "Resource")

	return resourceInfo{
		QualifiedName:     qualifiedName,
		ResourcePath:      resourcePath,
		ListType:          base + "List",
		ResourceType:      resourceType,
		NameParameterType: base + "NameParameter",
		NameParameterName: lowerFirst(base) + "Name",
	}
}

func lowerFirst(s string) string {
	sb := strings.Builder{}
	for i, r := range s {
		if i == 0 {
			sb.WriteRune(unicode.ToLower(r))
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

func (r resourceInfo) BasePath() string {
	if r.IsApplication() {
		return ApplicationBasePath
	}
	return ResourceBasePath
}

func (r resourceInfo) IsApplication() bool {
	return r.QualifiedName == ApplicationType
}

// Load a resource boilerplate schema for a given type.
func LoadResourceBoilerplateSchemaForType(r resourceInfo) (*Schema, error) {
	b := &bytes.Buffer{}
	err := boilerplateTemplate.Execute(b, r)
	if err != nil {
		return nil, err
	}
	s, err := LoadBytes(b.Bytes())
	if err != nil {
		return nil, err
	}
	s.InlineAllRefs()
	return s, nil
}
