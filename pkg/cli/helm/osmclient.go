// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/output"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/storage/driver"
)

const (
	osmReleaseName     = "osm"
	osmHelmRepo        = "https://openservicemesh.github.io/osm"
	OsmSystemNamespace = "osm-system"
)

// double check this 
type osmOptions struct {
	ChartPath              string
	ChartVersion           string
	Image                  string
}