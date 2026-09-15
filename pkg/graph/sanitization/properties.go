// Package sanitization removes sensitive values from application graph properties.
package sanitization

import "strings"

const containersResourceType = "Radius.Compute/containers"

// OmitContainerEnvironment returns a deep copy of properties with every container environment map omitted.
func OmitContainerEnvironment(resourceType string, properties map[string]any) map[string]any {
	if properties == nil {
		return nil
	}

	clonedProperties := cloneMap(properties)
	if !strings.EqualFold(resourceType, containersResourceType) {
		return clonedProperties
	}

	containers, ok := clonedProperties["containers"].(map[string]any)
	if !ok {
		return clonedProperties
	}

	for _, containerValue := range containers {
		container, ok := containerValue.(map[string]any)
		if !ok {
			continue
		}
		delete(container, "env")
	}

	return clonedProperties
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = cloneValue(item)
	}
	return cloned
}

func cloneValue(value any) any {
	switch typedValue := value.(type) {
	case map[string]any:
		return cloneMap(typedValue)
	case []any:
		cloned := make([]any, len(typedValue))
		for index, item := range typedValue {
			cloned[index] = cloneValue(item)
		}
		return cloned
	default:
		return value
	}
}
