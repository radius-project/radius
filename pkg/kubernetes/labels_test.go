// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"testing"
)

func TestConvertResourceTypeToLabelValue(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		want         string
	}{
		{
			name:         "valid-resource-type",
			resourceType: "Applications.Core/containers",
			want:         "Applications.Core-containers",
		},
		{
			name:         "invalid-resource-type",
			resourceType: "Applications.Core.containers",
			want:         "Applications.Core.containers",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertResourceTypeToLabelValue(tt.resourceType); got != tt.want {
				t.Errorf("ConvertResourceTypeToLabelValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertLabelToResourceType(t *testing.T) {
	tests := []struct {
		name       string
		labelValue string
		want       string
	}{
		{
			name:       "valid-label-value",
			labelValue: "applications.core-containers",
			want:       "applications.core/containers",
		},
		{
			name:       "invalid-label-value",
			labelValue: "Applications.Core.containers",
			want:       "Applications.Core.containers",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertLabelToResourceType(tt.labelValue); got != tt.want {
				t.Errorf("ConvertLabelToResourceType() = %v, want %v", got, tt.want)
			}
		})
	}
}
