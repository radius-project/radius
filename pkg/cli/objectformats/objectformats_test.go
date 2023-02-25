// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package objectformats

import (
	"bytes"
	"testing"

	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func Test_EnvTableFormat(t *testing.T) {
	obj := v20220315privatepreview.EnvironmentResource{
		Name: to.Ptr("test_env_resource"),
	}

	buffer := &bytes.Buffer{}
	err := output.Write(output.FormatTable, obj, buffer, GetGenericEnvironmentTableFormat())
	require.NoError(t, err)

	expected := "NAME\ntest_env_resource\n"
	require.Equal(t, expected, buffer.String())
}
