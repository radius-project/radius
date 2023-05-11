/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package setup

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type ChartArgs struct {
	Reinstall    bool
	ChartPath    string
	AppCoreImage string
	AppCoreTag   string
	UcpImage     string
	UcpTag       string

	// PublicEndpointOverride is used to define the public endpoint of the Kubernetes cluster
	// for display purposes. This is useful when the the actual public IP address of a cluster's ingress
	// is not a routable IP. This comes up all of the time for a local cluster.
	PublicEndpointOverride string

	// Values is a string consisting of list of values to pass to the Helm chart which would be used to override values in the chart.
	Values string
}

// RegisterPersistentChartArgs registers the CLI arguments used for our Helm chart.
func RegisterPersistentChartArgs(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("reinstall", false, "Specify to force reinstallation of Radius")
	cmd.PersistentFlags().String("chart", "", "Specify a file path to a helm chart to install Radius from")
	cmd.PersistentFlags().String("image", "", "Specify the radius controller image to use")
	cmd.PersistentFlags().String("tag", "", "Specify the radius controller tag to use")
	cmd.PersistentFlags().String("appcore-image", "", "Specify Application.Core RP image to use")
	cmd.PersistentFlags().String("appcore-tag", "", "Specify Application.Core RP image tag to use")
	cmd.PersistentFlags().String("ucp-image", "", "Specify the UCP image to use")
	cmd.PersistentFlags().String("ucp-tag", "", "Specify the UCP tag to use")
	cmd.PersistentFlags().String("public-endpoint-override", "", "Specify the public IP address or hostname of the Kubernetes cluster. It must be in the format: <hostname>[:<port>]. Ex: 'localhost:9000'")
	cmd.PersistentFlags().String("set", "", "Set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)")
}

// ParseChartArgs the arguments we provide for installation of the Helm chart.
func ParseChartArgs(cmd *cobra.Command) (*ChartArgs, error) {
	reinstall, err := cmd.Flags().GetBool("reinstall")
	if err != nil {
		return nil, err
	}
	chartPath, err := cmd.Flags().GetString("chart")
	if err != nil {
		return nil, err
	}
	appcoreImage, err := cmd.Flags().GetString("appcore-image")
	if err != nil {
		return nil, err
	}
	appcoreTag, err := cmd.Flags().GetString("appcore-tag")
	if err != nil {
		return nil, err
	}
	ucpImage, err := cmd.Flags().GetString("ucp-image")
	if err != nil {
		return nil, err
	}
	ucpTag, err := cmd.Flags().GetString("ucp-tag")
	if err != nil {
		return nil, err
	}
	values, err := cmd.Flags().GetString("set")
	if err != nil {
		return nil, err
	}

	publicEndpointOverride, err := cmd.Flags().GetString("public-endpoint-override")
	if err != nil {
		return nil, err
	} else if strings.HasPrefix(publicEndpointOverride, "http://") || strings.HasPrefix(publicEndpointOverride, "https://") {
		return nil, fmt.Errorf("a URL is not accepted here. Please specify the public endpoint override in the form <hostname>[:<port>]. Ex: 'localhost:9000'")
	}

	return &ChartArgs{
		Reinstall:              reinstall,
		ChartPath:              chartPath,
		AppCoreImage:           appcoreImage,
		AppCoreTag:             appcoreTag,
		UcpImage:               ucpImage,
		UcpTag:                 ucpTag,
		PublicEndpointOverride: publicEndpointOverride,
		Values:                 values,
	}, nil
}
