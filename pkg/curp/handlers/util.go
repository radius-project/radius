// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"github.com/Azure/radius/pkg/curp/db"
	"github.com/Azure/radius/pkg/workloads"
)

// mergeProperties combines properties from a resource definition and a potentially existing resource.
// This is useful for cases where deploying a resource results in storage of generated values like names.
// By merging properties, the caller gets to see those values and reuse them.
func mergeProperties(resource workloads.WorkloadResource, existing *db.DeploymentResource) map[string]string {
	properties := resource.Resource.(map[string]string)
	if properties == nil {
		properties = map[string]string{}
	}

	if existing == nil {
		return properties
	}

	for k, v := range existing.Properties {
		_, ok := properties[k]
		if !ok {
			properties[k] = v
		}
	}

	return properties
}
