// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/require"
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

func TestNormalizeResoureName(t *testing.T) {
	nameTests := []struct {
		in    string
		out   string
		panic bool
	}{
		{
			in:  "resource",
			out: "resource",
		},
		{
			in:  "Resource",
			out: "resource",
		},
		{
			in:    "panic_",
			out:   "",
			panic: true,
		},
		{
			in:  "",
			out: "",
		},
	}

	for _, tt := range nameTests {
		t.Run(tt.in, func(t *testing.T) {
			if tt.panic {
				require.Panics(t, func() {
					NormalizeResourceName(tt.in)
				})
			} else {
				require.Equal(t, tt.out, NormalizeResourceName(tt.in))
			}
		})
	}
}

func TestNormalizeResoureNameDapr(t *testing.T) {
	nameTests := []struct {
		in    string
		out   string
		panic bool
	}{
		{
			in:  "pub.sub",
			out: "pub.sub",
		},
		{
			in:  "pub-sub",
			out: "pub-sub",
		},
		{
			in:  "Resource",
			out: "resource",
		},
		{
			in:    "pub_sub",
			out:   "pub_sub",
			panic: true,
		},
	}

	for _, tt := range nameTests {
		t.Run(tt.in, func(t *testing.T) {
			if tt.panic {
				require.Panics(t, func() {
					NormalizeDaprResourceName(tt.in)
				})
			} else {
				require.Equal(t, tt.out, NormalizeDaprResourceName(tt.in))
			}
		})
	}
}
