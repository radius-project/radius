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
	Long: `Downloads the bicep compiler and manifest-to-bicep extension locally.

This command supports custom download URLs and versions for air-gapped environments.

Environment Variables:
  RAD_BICEP                        Override the bicep binary installation path
  RAD_MANIFEST_TO_BICEP_EXTENSION  Override the manifest-to-bicep extension installation path

Examples:
  # Download latest versions from default sources
  rad bicep download
  
  # Download specific versions
  rad bicep download --bicep-download-version v0.21.1 --manifest-to-bicep-extension-download-version v0.3.0
  
  # Use custom URLs for air-gapped environments
  rad bicep download --bicep-download-url https://internal.company.com/bicep/releases --manifest-to-bicep-extension-download-url https://internal.company.com/manifest-extension/releases`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.LogInfo("Downloading Bicep for channel %s...", version.Channel())
		
		options := bicep.DownloadOptions{
			BicepURL:                         bicepDownloadURL,
			BicepVersion:                     bicepDownloadVersion,
			ManifestToBicepExtensionURL:      manifestToBicepExtensionDownloadURL,
			ManifestToBicepExtensionVersion:  manifestToBicepExtensionDownloadVersion,
		}
		
		err := bicep.DownloadBicepTools(options)
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
