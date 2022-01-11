// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cli_test

import (
	"regexp"
	"testing"

	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/test/kubernetestest"
	"github.com/project-radius/radius/test/radcli"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

// These tests just verify our commands for interacting with the local dev environment
func Test_CLI(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	options := kubernetestest.NewTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	t.Run("rad env status", func(t *testing.T) {
		output, err := cli.EnvStatus(ctx)
		require.NoError(t, err)

		expected := regexp.MustCompile(`NODES\s+REGISTRY\s+INGRESS \(HTTP\)\s+INGRESS \(HTTPS\)\s*Ready \(2\/2\)\s+localhost:\d+\s+http:\/\/localhost:\d+\s+https:\/\/localhost:\d+`)
		require.Regexp(t, expected, objectformats.TrimSpaceMulti(output))
	})
}
