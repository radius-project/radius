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

	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/version"
)

const (
	radiusReleaseName     = "radius"
	radiusHelmRepo        = "https://radius.azurecr.io/helm/v1/repo"
	RadiusSystemNamespace = "radius-system"
)

type RadiusOptions struct {
	Reinstall              bool
	ChartPath              string
	ChartVersion           string
	Image                  string
	Tag                    string
	AppCoreImage           string
	AppCoreTag             string
	UCPImage               string
	UCPTag                 string
	DEImage                string
	DETag                  string
	PublicEndpointOverride string
	AzureProvider          *azure.Provider
	AWSProvider            *aws.Provider
	Values                 string
}

func ApplyRadiusHelmChart(options RadiusOptions, kubeContext string) (bool, error) {
	// For capturing output from helm.
	var helmOutput strings.Builder
	alreadyInstalled := false
	namespace := RadiusSystemNamespace
	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	}

	helmConf, err := HelmConfig(&helmOutput, &flags)
	if err != nil {
		return false, fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	var helmChart *chart.Chart
	if options.ChartPath == "" {
		helmChart, err = helmChartFromContainerRegistry(options.ChartVersion, helmConf, radiusHelmRepo, radiusReleaseName)
	} else {
		helmChart, err = loader.Load(options.ChartPath)
	}

	if err != nil {
		return false, fmt.Errorf("failed to load helm chart, err: %w, helm output: %s", err, helmOutput.String())
	}

	// TODO: refactor this to use the addChartValues function
	err = AddRadiusValues(helmChart, &options)
	if err != nil {
		return false, fmt.Errorf("failed to add radius values, err: %w, helm output: %s", err, helmOutput.String())
	}

	if options.AWSProvider != nil {
		err = addAWSProviderValues(helmChart, options.AWSProvider)
		if err != nil {
			return false, fmt.Errorf("failed to add aws provider values, err: %w, helm output: %s", err, helmOutput.String())
		}
	}

	// https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you
	// TODO: Apply CRDs because Helm doesn't upgrade CRDs for you.
	// https://github.com/project-radius/radius/issues/712
	// We need the CRDs to be public to do this (or consider unpacking the chart
	// for the CRDs)

	histClient := helm.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists
	version := version.Version()

	// See: https://github.com/helm/helm/blob/281380f31ccb8eb0c86c84daf8bcbbd2f82dc820/cmd/helm/upgrade.go#L99
	// The upgrade client's install option doesn't seem to work, so we have to check the history of releases manually
	// and invoke the install client.
	_, err = histClient.Run(radiusReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("Installing Radius version %s control plane to namespace: %s", version, RadiusSystemNamespace)

		err = runRadiusHelmInstall(helmConf, helmChart)
		if err != nil {
			return false, fmt.Errorf("failed to run radius helm install, err: \n%w\nhelm output:\n%s", err, helmOutput.String())
		}
	} else if options.Reinstall {
		output.LogInfo("Reinstalling Radius version %s control plane to namespace: %s", version, RadiusSystemNamespace)

		err = runRadiusHelmUpgrade(helmConf, radiusReleaseName, helmChart)
		if err != nil {
			return false, fmt.Errorf("failed to run radius helm upgrade, err: \n%w\nhelm output:\n%s", err, helmOutput.String())
		}
	} else if err == nil {
		alreadyInstalled = true
		output.LogInfo("Found existing Radius installation. Use '--reinstall' to force reinstallation.")
	}
	return alreadyInstalled, err
}

func GetAzProvider(options RadiusOptions, kubeContext string) (*azure.Provider, error) {

	var helmOutput strings.Builder

	namespace := RadiusSystemNamespace
	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	}

	helmConf, err := HelmConfig(&helmOutput, &flags)
	if err != nil {
		return nil, fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	histClient := helm.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists
	rel, err := histClient.Run(radiusReleaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get helm config, err: %w", err)
	}

	if len(rel) == 0 {
		return nil, nil
	}
	cfg := rel[0].Config

	_, ok := cfg["global"]
	if !ok {
		return nil, nil
	}
	global := cfg["global"].(map[string]any)

	_, ok = global["rp"]
	if !ok {
		return nil, nil
	}
	rp := global["rp"].(map[string]any)

	_, ok = rp["provider"]
	if !ok {
		return nil, nil
	}
	provider := rp["provider"].(map[string]any)

	_, ok = provider["azure"]
	if !ok {
		return nil, nil
	}
	azureProvider := provider["azure"].(map[string]any)

	var azProvider azure.Provider

	subscriptionId, ok := azureProvider["subscriptionId"]
	if !ok {
		return nil, nil
	}
	resourceGroup, ok := azureProvider["resourceGroup"]
	if !ok {
		return nil, nil
	}

	azProvider.SubscriptionID = subscriptionId.(string)
	azProvider.ResourceGroup = resourceGroup.(string)
	return &azProvider, nil

}

func runRadiusHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = radiusReleaseName
	installClient.Namespace = RadiusSystemNamespace
	installClient.CreateNamespace = true
	installClient.Wait = true
	installClient.Timeout = installTimeout
	return runInstall(installClient, helmChart)
}

func runRadiusHelmUpgrade(helmConf *helm.Configuration, releaseName string, helmChart *chart.Chart) error {
	installClient := helm.NewUpgrade(helmConf)
	installClient.Namespace = RadiusSystemNamespace
	installClient.Wait = true
	installClient.Timeout = installTimeout
	installClient.Recreate = true //force recreating radius pods on adding or modfying azprovider
	return runUpgrade(installClient, releaseName, helmChart)
}

func AddRadiusValues(helmChart *chart.Chart, options *RadiusOptions) error {
	values := helmChart.Values

	// TODO: clean up below code using options, for now retain it since CI/CD uses old rad env init with options.
	_, ok := values["rp"]
	if !ok {
		values["rp"] = make(map[string]any)
	}
	radiusRP := values["rp"].(map[string]any)

	if options.AppCoreImage != "" {
		radiusRP["image"] = options.AppCoreImage
	}
	if options.AppCoreTag != "" {
		radiusRP["tag"] = options.AppCoreTag
	}
	if options.PublicEndpointOverride != "" {
		radiusRP["publicEndpointOverride"] = options.PublicEndpointOverride
	}
	_, ok = values["ucp"]
	if !ok {
		values["ucp"] = make(map[string]any)
	}
	ucp := values["ucp"].(map[string]any)

	if options.UCPImage != "" {
		ucp["image"] = options.UCPImage
	}
	if options.UCPTag != "" {
		ucp["tag"] = options.UCPTag
	}

	_, ok = values["de"]
	if !ok {
		values["de"] = make(map[string]any)
	}
	de := values["de"].(map[string]any)

	if options.DEImage != "" {
		de["image"] = options.DEImage
	}

	if options.DETag != "" {
		de["tag"] = options.DETag
	}

	err := strvals.ParseInto(options.Values, values)
	if err != nil {
		return err
	}
	return nil
}

func addAWSProviderValues(helmChart *chart.Chart, awsProvider *aws.Provider) error {
	if awsProvider == nil {
		return nil
	}
	values := helmChart.Values

	_, ok := values["ucp"]
	if !ok {
		values["ucp"] = make(map[string]any)
	}
	ucp := values["ucp"].(map[string]any)

	_, ok = ucp["provider"]
	if !ok {
		ucp["provider"] = make(map[string]any)
	}
	provider := ucp["provider"].(map[string]any)

	_, ok = provider["aws"]
	if !ok {
		provider["aws"] = make(map[string]any)
	}
	aws := provider["aws"].(map[string]any)

	aws["region"] = awsProvider.TargetRegion
	return nil
}

func RunRadiusHelmUninstall(helmConf *helm.Configuration) error {
	output.LogInfo("Uninstalling Radius from namespace: %s", RadiusSystemNamespace)
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = uninstallTimeout
	uninstallClient.Wait = true
	_, err := uninstallClient.Run(radiusReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("Radius not found")
		return nil
	}
	return err
}
