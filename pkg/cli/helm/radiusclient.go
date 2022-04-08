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
	radiusReleaseName     = "radius"
	radiusHelmRepo        = "https://radius.azurecr.io/helm/v1/repo"
	RadiusSystemNamespace = "radius-system"
)

type RadiusOptions struct {
	ChartPath     string
	ChartVersion  string
	Image         string
	Tag           string
	AzureProvider *azure.Provider
}

func ApplyRadiusHelmChart(options RadiusOptions) error {
	// For capturing output from helm.
	var helmOutput strings.Builder

	helmConf, err := HelmConfig(RadiusSystemNamespace, helmOutput)
	if err != nil {
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput.String())
	}

	var helmChart *chart.Chart
	if options.ChartPath == "" {
		helmChart, err = helmChartFromContainerRegistry(options.ChartVersion, helmConf, radiusHelmRepo, radiusReleaseName)
	} else {
		helmChart, err = loader.Load(options.ChartPath)
	}

	if err != nil {
		return fmt.Errorf("failed to load helm chart, err: %w, helm output: %s", err, helmOutput.String())
	}

	err = addRadiusValues(helmChart, options.Image, options.Tag)
	if err != nil {
		return fmt.Errorf("failed to add radius values, err: %w, helm output: %s", err, helmOutput.String())
	}

	if options.AzureProvider != nil {
		err = addAzureProviderValues(helmChart, options.AzureProvider)
		if err != nil {
			return fmt.Errorf("failed to add azure values, err: %w, helm output: %s", err, helmOutput.String())
		}
	}

	// https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you
	// TODO: Apply CRDs because Helm doesn't upgrade CRDs for you.
	// https://github.com/project-radius/radius/issues/712
	// We need the CRDs to be public to do this (or consider unpacking the chart
	// for the CRDs)

	histClient := helm.NewHistory(helmConf)
	histClient.Max = 1 // Only need to check if at least 1 exists

	// See: https://github.com/helm/helm/blob/281380f31ccb8eb0c86c84daf8bcbbd2f82dc820/cmd/helm/upgrade.go#L99
	// The upgrade client's install option doesn't seem to work, so we have to check the history of releases manually
	// and invoke the install client.
	_, err = histClient.Run(radiusReleaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		output.LogInfo("Installing new Radius Kubernetes environment to namespace: %s", RadiusSystemNamespace)

		err = runRadiusHelmInstall(helmConf, helmChart)
		if err != nil {
			return fmt.Errorf("failed to run helm install, err: %w, helm output: %s", err, helmOutput.String())
		}
	} else if err == nil {
		output.LogInfo("Found existing Radius Kubernetes environment")
	}

	return err
}

func runRadiusHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = radiusReleaseName
	installClient.Namespace = RadiusSystemNamespace
	installClient.Wait = true
	installClient.Timeout = installTimeout
	_, err := installClient.Run(helmChart, helmChart.Values)
	return err
}

func addRadiusValues(helmChart *chart.Chart, rpImage string, containerTag string) error {
	values := helmChart.Values

	_, ok := values["global"]
	if !ok {
		values["global"] = make(map[string]interface{})
	}
	global := values["global"].(map[string]interface{})

	_, ok = global["radius"]
	if !ok {
		global["radius"] = make(map[string]interface{})
	}

	radius := global["radius"].(map[string]interface{})

	if containerTag != "" {
		radius["tag"] = containerTag
	}

	_, ok = global["rp"]
	if !ok {
		global["rp"] = make(map[string]interface{})
	}

	rp := global["rp"].(map[string]interface{})

	if rpImage != "" {
		rp["container"] = rpImage
	}

	if containerTag != "" {
		rp["tag"] = containerTag
		radius["tag"] = containerTag
	}

	return nil
}

func addAzureProviderValues(helmChart *chart.Chart, azureProvider *azure.Provider) error {
	if azureProvider == nil {
		return nil
	}
	values := helmChart.Values

	_, ok := values["global"]
	if !ok {
		values["global"] = make(map[string]interface{})
	}
	global := values["global"].(map[string]interface{})

	_, ok = global["rp"]
	if !ok {
		global["rp"] = make(map[string]interface{})
	}
	rp := global["rp"].(map[string]interface{})

	_, ok = rp["provider"]
	if !ok {
		rp["provider"] = make(map[string]interface{})
	}
	provider := rp["provider"].(map[string]interface{})

	_, ok = provider["azure"]
	if !ok {
		provider["azure"] = make(map[string]interface{})
	}

	azure := provider["azure"].(map[string]interface{})

	azure["subscriptionId"] = azureProvider.SubscriptionID
	azure["resourceGroup"] = azureProvider.ResourceGroup

	if azureProvider.ServicePrincipal != nil {
		_, ok = azure["servicePrincipal"]
		if !ok {
			azure["servicePrincipal"] = make(map[string]interface{})
		}
		azure["servicePrincipal"] = map[string]interface{}{
			"clientId":     azureProvider.ServicePrincipal.ClientID,
			"clientSecret": azureProvider.ServicePrincipal.ClientSecret,
			"tenantId":     azureProvider.ServicePrincipal.TenantID,
		}
	} else if azureProvider.PodIdentitySelector != nil {
		azure["podidentity"] = *azureProvider.PodIdentitySelector
	}

	if azureProvider.AKS != nil {
		_, ok = rp["aks"]
		if !ok {
			rp["aks"] = make(map[string]interface{})
		}

		aks := rp["aks"].(map[string]interface{})
		aks["clusterName"] = azureProvider.AKS.ClusterName
		aks["subscriptionId"] = azureProvider.AKS.SubscriptionID
		aks["resourceGroup"] = azureProvider.AKS.ResourceGroup
	}

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
