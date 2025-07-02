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

package cmd

import (
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/version"
	"github.com/spf13/cobra"
)

var (
	bicepDownloadURL                           string
	bicepDownloadVersion                       string
	manifestToBicepExtensionDownloadURL        string
	manifestToBicepExtensionDownloadVersion    string
)

var bicepDownloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download the bicep compiler and manifest-to-bicep extension",
	Long:  `Downloads the bicep compiler and manifest-to-bicep extension locally`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.LogInfo("Downloading Bicep for channel %s...", version.Channel())
		
		options := bicep.DownloadOptions{
			BicepURL:                         bicepDownloadURL,
			BicepVersion:                     bicepDownloadVersion,
			ManifestToBicepExtensionURL:      manifestToBicepExtensionDownloadURL,
			ManifestToBicepExtensionVersion:  manifestToBicepExtensionDownloadVersion,
		}
		
		err := bicep.DownloadBicepWithOptions(options)
		return err
	},
}

func init() {
	bicepCmd.AddCommand(bicepDownloadCmd)
	
	bicepDownloadCmd.Flags().StringVar(&bicepDownloadURL, "bicep-download-url", "", "Custom URL for downloading the bicep compiler")
	bicepDownloadCmd.Flags().StringVar(&bicepDownloadVersion, "bicep-download-version", "", "Specific version of the bicep compiler to download")
	bicepDownloadCmd.Flags().StringVar(&manifestToBicepExtensionDownloadURL, "manifest-to-bicep-extension-download-url", "", "Custom URL for downloading the manifest-to-bicep extension")
	bicepDownloadCmd.Flags().StringVar(&manifestToBicepExtensionDownloadVersion, "manifest-to-bicep-extension-download-version", "", "Specific version of the manifest-to-bicep extension to download")
}
