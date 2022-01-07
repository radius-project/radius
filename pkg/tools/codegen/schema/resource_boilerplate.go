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

	"github.com/project-radius/radius/pkg/radrp/schema"
)

var (
	ResourceBasePath    = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/{applicationName}"
	ApplicationBasePath = "/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3"

	//go:embed resource_boilerplate.json
	boilerplateTemplateText string
	boilerplateTemplate     = template.Must(template.New("resourceBoilerplateTemplate").Parse(boilerplateTemplateText))
)

type resourceInfo struct {
	BaseName      string
	QualifiedName string
	ResourcePath  string
}

func newResourceInfo(qualifiedName, resourcePath string) resourceInfo {
	tokens := strings.Split(resourcePath, "/")
	resourceType := tokens[len(tokens)-1]
	base := strings.TrimSuffix(resourceType, "Resource")
	if base == "Radius" {
		// We want RadiusResourceList, RadiusResourceNameParameter
		// instead of RadiusList, RadiusNameParameter
		base = "RadiusResource"
	}
	return resourceInfo{
		BaseName:      base,
		QualifiedName: qualifiedName,
		ResourcePath:  resourcePath,
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
	return schema.IsApplicationResource(r.BaseName)
}

func (r resourceInfo) IsGenericResource() bool {
	return schema.IsGenericResource(r.BaseName)
}

func (r resourceInfo) ListType() string {
	return r.BaseName + "List"
}

func (r resourceInfo) NameParameterType() string {
	return r.BaseName + "NameParameter"
}

func (r resourceInfo) NameParameterName() string {
	return lowerFirst(r.BaseName) + "Name"
}

func (r resourceInfo) TypeParameterType() string {
	return schema.GenericResourceType + "TypeParameter"
}

func (r resourceInfo) TypeParameterName() string {
	return lowerFirst(schema.GenericResourceType) + "Type"
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
