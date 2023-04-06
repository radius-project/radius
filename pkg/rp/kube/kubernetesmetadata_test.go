// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kube

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	envKey1      = "env.ann1"
	envVal1      = "env.annval1"
	objMetaKey1  = "obj.ann1"
	objMetaVal1  = "obj.annval1"
	specKey1     = "spec.ann1"
	specVal1     = "spec.val1"
	reservedKey1 = "radius.dev/input1"
	reservedVal1 = "reserved.val1"
	appKey1      = "app.lbl1"
	appVal1      = "env.lblval1"
	overrideKey1 = "test.ann1"
	overrideVal1 = "override.app.annval1"
	overrideVal2 = "override.app.lblval1"
)

func Test_Render_WithEnvironment_KubernetesMetadata(t *testing.T) {
	// Setup
	envData := map[string]string{
		envKey1: envVal1,
	}
	appData := map[string]string{
		appKey1:      appVal1,
		overrideKey1: overrideVal1,
	}
	customInput := map[string]string{
		overrideKey1: overrideVal2,
		reservedKey1: reservedVal1,
	}
	objectMetadata := map[string]string{
		objMetaKey1: objMetaVal1,
	}
	specData := map[string]string{
		specKey1: specVal1,
	}
	expectedMetadataMap := map[string]string{
		envKey1:      envVal1,
		objMetaKey1:  objMetaVal1,
		appKey1:      appVal1,
		overrideKey1: overrideVal2,
	}
	expectedSpecMap := map[string]string{
		envKey1:      envVal1,
		appKey1:      appVal1,
		overrideKey1: overrideVal2,
		specKey1:     specVal1,
	}

	input := Metadata{
		EnvData:        envData,
		AppData:        appData,
		Input:          customInput,
		ObjectMetadata: objectMetadata,
		SpecData:       specData,
	}

	// Testing for cascading, overriding, and reserved keys
	metaMap, specMap := input.Merge(context.Background())

	// Verify
	require.Equal(t, metaMap, expectedMetadataMap)
	require.Equal(t, specMap, expectedSpecMap)
}
