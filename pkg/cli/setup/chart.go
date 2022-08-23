// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package setup

import "github.com/spf13/cobra"

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
}

// RegisterPersistantChartArgs registers the CLI arguments used for our Helm chart.
func RegisterPersistantChartArgs(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("reinstall", false, "Specify to force reinstallation of Radius")
	cmd.PersistentFlags().String("chart", "", "Specify a file path to a helm chart to install Radius from")
	cmd.PersistentFlags().String("image", "", "Specify the radius controller image to use")
	cmd.PersistentFlags().String("tag", "", "Specify the radius controller tag to use")
	cmd.PersistentFlags().String("appcore-image", "", "Specify Application.Core RP image to use")
	cmd.PersistentFlags().String("appcore-tag", "", "Specify Application.Core RP image tag to use")
	cmd.PersistentFlags().String("ucp-image", "", "Specify the UCP image to use")
	cmd.PersistentFlags().String("ucp-tag", "", "Specify the UCP tag to use")
	cmd.PersistentFlags().String("public-endpoint-override", "", "Specify the public IP address or hostname of the Kubernetes cluster. It must be in the format: <protocol>://<hostname>[:<port>]. Ex: 'http://localhost:9000'")
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
	publicEndpointOverride, err := cmd.Flags().GetString("public-endpoint-override")
	if err != nil {
		return nil, err
	}

	return &ChartArgs{
		Reinstall:              reinstall,
		ChartPath:              chartPath,
		AppCoreImage:           appcoreImage,
		AppCoreTag:             appcoreTag,
		UcpImage:               ucpImage,
		UcpTag:                 ucpTag,
		PublicEndpointOverride: publicEndpointOverride,
	}, nil
}
