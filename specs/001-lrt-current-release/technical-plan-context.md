# Context for the plan

This document provides additional context and reference information to support the technical planning of the "Long-Running Tests Use Current Release" feature. It includes details about the Radius CLI commands relevant to the feature, as well as a breakdown of the existing GitHub Actions workflow steps for the long-running Azure tests. This document is provided to the Spec Kit `/plan` prompt.

## Radius CLI

### `rad version` command

The `rad version` command displays the installed version of the Radius CLI and the version of the Radius control plane installed on the connected Kubernetes cluster. An example is below:

```shell
$ rad version
CLI Version Information:
RELEASE   VERSION   BICEP     COMMIT
0.54.0    v0.54.0   0.39.26   f06410904c8b92bcc3aaa1f1ed6450981e510107

Control Plane Information:
STATUS     VERSION
Installed  0.54.0
```

There is also an option to output the version information in JSON format, however the current version of Radius outputs invalid JSON, so the updates to the workflow will need to parse the output accordingly, either using this invalid json, or using the text version above. An example of the JSON output is below:

```shell
$ rad version --output json
{
  "release": "0.54.0",
  "version": "v0.54.0",
  "bicep": "0.39.26",
  "commit": "f06410904c8b92bcc3aaa1f1ed6450981e510107"
}
{
  "version": "0.54.0",
  "status": "Installed"
}
```

Another option is to specify only the CLI version number, which outputs a json version for the CLI version only. However, this option does not display the control plane version.

```shell
$ rad version --cli --output json
{
  "release": "0.54.0",
  "version": "v0.54.0",
  "bicep": "0.39.26",
  "commit": "f06410904c8b92bcc3aaa1f1ed6450981e510107"
}
```

This is the help text for the `rad version` command:

```shell
$ rad version --help
Display version information for the rad CLI installed on your machine and the Radius Control Plane running on your cluster.
By default this shows all available version information.

Usage:
  rad version [flags]

Examples:
# Show all version information
rad version

# Show only the CLI version
rad version --cli

Flags:
      --cli    Use this flag to only show the rad CLI version
  -h, --help   help for version

Global Flags:
      --config string   config file (default "$HOME/.rad/config.yaml")
  -o, --output string   output format (supported formats are json, table) (default "table")
```

### `rad upgrade kubernetes` command

Radius control plane upgrades are handled by the `rad upgrade kubernetes` command. This command upgrades the Radius control plane installed on the connected Kubernetes cluster to the specified version. If no version is specified, it upgrades to the version that matches the installed Radius CLI version.

The help text for the `rad upgrade kubernetes` command is below:

```shell
$ rad upgrade kubernetes --help
Upgrade Radius in a Kubernetes cluster using the Radius Helm chart.
This command upgrades the Radius control plane in the cluster associated with the active workspace.
To upgrade Radius in a different cluster, switch to the appropriate workspace first using 'rad workspace switch'.

The upgrade process includes preflight checks to ensure the cluster is ready for upgrade.
Preflight checks include:
- Kubernetes connectivity and permissions
- Helm connectivity and Radius installation status
- Version compatibility validation
- Cluster resource availability
- Custom configuration parameter validation

Radius is installed in the 'radius-system' namespace. For more information visit https://docs.radapp.io/concepts/technical/architecture/.

Usage:
  rad upgrade kubernetes [flags]

Examples:
# Upgrade Radius in the cluster of the active workspace
rad upgrade kubernetes

# Check which workspace is active
rad workspace show

# Switch to a different workspace before upgrading
rad workspace switch myworkspace
rad upgrade kubernetes

# Upgrade Radius with custom configuration
rad upgrade kubernetes --set key=value

# Upgrade Radius with a custom container registry
# Images will be pulled as: myregistry.azurecr.io/controller, myregistry.azurecr.io/ucpd, etc.
rad upgrade kubernetes --set global.imageRegistry=myregistry.azurecr.io

# Upgrade Radius to a specific version tag for all components
rad upgrade kubernetes --set global.imageTag=0.48

# Upgrade Radius with custom registry and tag
# Images will be pulled as: myregistry.azurecr.io/controller:0.48, etc.
rad upgrade kubernetes --set global.imageRegistry=myregistry.azurecr.io,global.imageTag=0.48

# Upgrade Radius with private registry and image pull secrets
# Note: Secret must be created in radius-system namespace first
rad upgrade kubernetes --set global.imageRegistry=myregistry.azurecr.io --set-string 'global.imagePullSecrets[0].name=regcred'

# Upgrade Radius with multiple image pull secrets for different registries
rad upgrade kubernetes --set-string 'global.imagePullSecrets[0].name=azure-cred' \
                       --set-string 'global.imagePullSecrets[1].name=aws-cred'

# Upgrade to a specific version
rad upgrade kubernetes --version 0.47.0

# Upgrade to the latest available version
rad upgrade kubernetes --version latest

# Upgrade Radius with values from a file
rad upgrade kubernetes --set-file global.rootCA.cert=/path/to/rootCA.crt

# Skip preflight checks (not recommended)
rad upgrade kubernetes --skip-preflight

# Run only preflight checks without upgrading
rad upgrade kubernetes --preflight-only

# Upgrade Radius using a Helm chart from specified file path
rad upgrade kubernetes --chart /root/radius/deploy/Chart


Flags:
      --chart string           Specify a file path to a helm chart to upgrade Radius from
  -h, --help                   help for kubernetes
      --kubecontext string     The Kubernetes context to use, will use the default if unset
      --preflight-only         Run only preflight checks without performing the upgrade
      --set stringArray        Set values on the command line (can specify multiple or separate values with commas: key1=val1,key2=val2)
      --set-file stringArray   Set values from files on the command line (can specify multiple or separate files with commas: key1=filename1,key2=filename2)
      --skip-preflight         Skip preflight checks before upgrade (not recommended)
      --version string         Specify the version to upgrade to (default: CLI version, use 'latest' for latest available)

Global Flags:
      --config string   config file (default "$HOME/.rad/config.yaml")
  -o, --output string   output format (supported formats are json, table) (default "table")
```

Here is an example of the command being used to attempt an upgrade in which the control plane is already at the latest version:

```shell
$ rad upgrade kubernetes
Current Radius version: 0.54.0
Target Radius version: 0.54.0
Running pre-flight checks...
  Running Kubernetes Connectivity...
    ✓ Connected (version: v1.31.5+k3s1) with sufficient permissions
  Running Helm Connectivity...
    ✓ Helm successfully connected to cluster and found Radius release (version: 0.54.0)
  Running Radius Installation...
    ✓ Radius is installed (version: 0.54.0), Contour is not installed (will be installed during upgrade)
  Running Version Compatibility...
    ✗ [ERROR] Target version is the same as current version
Error: preflight checks failed: pre-flight check 'Version Compatibility' failed: Target version is the same as current version

TraceId:  1fa4f0e0732bbd2f0aa98d4a27e90523
```

It may be useful for the GitHub workflow to test whether an upgrade is possible by running the `rad upgrade kubernetes --preflight-only` command first, and checking the output or exit code to determine if the upgrade can proceed.

```shell
$ rad upgrade kubernetes --preflight-only
Current Radius version: 0.54.0
Target Radius version: 0.54.0
Running pre-flight checks...
  Running Kubernetes Connectivity...
    ✓ Connected (version: v1.31.5+k3s1) with sufficient permissions
  Running Helm Connectivity...
    ✓ Helm successfully connected to cluster and found Radius release (version: 0.54.0)
  Running Radius Installation...
    ✓ Radius is installed (version: 0.54.0), Contour is not installed (will be installed during upgrade)
  Running Version Compatibility...
    ✗ [ERROR] Target version is the same as current version
Error: preflight checks failed: pre-flight check 'Version Compatibility' failed: Target version is the same as current version

TraceId:  9776424e46d2ec03f2bed0061f7620a0
```

## Long-running Azure Workflow Steps

This document summarizes the jobs and steps in the GitHub Actions workflow file for the long-running Azure tests and provides clickable VS Code links to each step.

File: [.github/workflows/long-running-azure.yaml](.github/workflows/long-running-azure.yaml)

### Jobs

- **build**: [Build Radius for test](.github/workflows/long-running-azure.yaml#L115)
- **tests**: [Run functional tests](.github/workflows/long-running-azure.yaml#L368)
- **report-failure**: [Report test failure](.github/workflows/long-running-azure.yaml#L762)

---

## build job steps

- [Restore the latest cached binaries](.github/workflows/long-running-azure.yaml#L133)
- [Skip build if build is still valid](.github/workflows/long-running-azure.yaml#L139)
- [Set up checkout target (scheduled, workflow_dispatch)](.github/workflows/long-running-azure.yaml#L161)
- [Set up checkout target (pull_request)](.github/workflows/long-running-azure.yaml#L169)
- [Generate ID for release](.github/workflows/long-running-azure.yaml#L178)
- [Checkout](.github/workflows/long-running-azure.yaml#L217)
- [Setup Go](.github/workflows/long-running-azure.yaml#L226)
- [Log the summary of build info for new version.](.github/workflows/long-running-azure.yaml#L234)
- [Login to Azure](.github/workflows/long-running-azure.yaml#L265)
- [Login to GitHub Container Registry](.github/workflows/long-running-azure.yaml#L273)
- [Build and Push container images](.github/workflows/long-running-azure.yaml#L280)
- [Upload CLI binary](.github/workflows/long-running-azure.yaml#L288)
- [Log the build result (success)](.github/workflows/long-running-azure.yaml#L296)
- [Log the build result (failure)](.github/workflows/long-running-azure.yaml#L302)
- [Log test Bicep recipe publish status](.github/workflows/long-running-azure.yaml#L308)
- [Move the latest binaries to cache](.github/workflows/long-running-azure.yaml#L314)
- [Store the latest binaries into cache](.github/workflows/long-running-azure.yaml#L329)
- [Publish UDT types](.github/workflows/long-running-azure.yaml#L336)
- [Publish Bicep Test Recipes](.github/workflows/long-running-azure.yaml#L348)
- [Log Bicep recipe publish status (success)](.github/workflows/long-running-azure.yaml#L358)
- [Log recipe publish status (failure)](.github/workflows/long-running-azure.yaml#L363)

---

## tests job steps

- [Get GitHub app token](.github/workflows/long-running-azure.yaml#L388)
- [Checkout](.github/workflows/long-running-azure.yaml#L406)
- [Checkout samples repo](.github/workflows/long-running-azure.yaml#L414)
- [Setup Go](.github/workflows/long-running-azure.yaml#L423)
- [Download rad CLI](.github/workflows/long-running-azure.yaml#L430)
- [Restore the latest cached binaries](.github/workflows/long-running-azure.yaml#L437)
- [Install rad CLI in bin](.github/workflows/long-running-azure.yaml#L444)
- [Login to Azure](.github/workflows/long-running-azure.yaml#L451)
- [Login to GitHub Container Registry](.github/workflows/long-running-azure.yaml#L458)
- [Create azure resource group - ${{ env.AZURE_TEST_RESOURCE_GROUP }}](.github/workflows/long-running-azure.yaml#L465)
- [Get kubeconf credential for AKS cluster](.github/workflows/long-running-azure.yaml#L477)
- [Restore skip-delete-resources-list](.github/workflows/long-running-azure.yaml#L486)
- [Clean up cluster](.github/workflows/long-running-azure.yaml#L494)
- [Download Bicep](.github/workflows/long-running-azure.yaml#L510)
- [Install gotestsum (test reporting tool)](.github/workflows/long-running-azure.yaml#L518)
- [Install Radius](.github/workflows/long-running-azure.yaml#L522)
- [Verify manifests are registered](.github/workflows/long-running-azure.yaml#L535)
- [Create a list of resources not to be deleted](.github/workflows/long-running-azure.yaml#L581)
- [Save list of resources not to be deleted](.github/workflows/long-running-azure.yaml#L586)
- [Configure Radius test workspace](.github/workflows/long-running-azure.yaml#L592)
- [Log radius installation status (failure)](.github/workflows/long-running-azure.yaml#L619)
- [Install Flux CLI and Source Controller](.github/workflows/long-running-azure.yaml#L624)
- [Install Git HTTP backend](.github/workflows/long-running-azure.yaml#L627)
- [Port-forward to Git server](.github/workflows/long-running-azure.yaml#L633)
- [Publish Terraform test recipes](.github/workflows/long-running-azure.yaml#L645)
- [Get OIDC Issuer from AKS cluster](.github/workflows/long-running-azure.yaml#L649)
- [Restore Bicep artifacts before running functional tests](.github/workflows/long-running-azure.yaml#L653)
- [Run functional tests](.github/workflows/long-running-azure.yaml#L662)
- [Collect Pod details](.github/workflows/long-running-azure.yaml#L696)
- [Upload container logs](.github/workflows/long-running-azure.yaml#L708)
- [Log radius e2e test status (success)](.github/workflows/long-running-azure.yaml#L715)
- [Log radius e2e test status (failure)](.github/workflows/long-running-azure.yaml#L720)
- [Login to Azure](.github/workflows/long-running-azure.yaml#L725)
- [Delete azure resource group - ${{ env.AZURE_TEST_RESOURCE_GROUP }}](.github/workflows/long-running-azure.yaml#L733)
- [Restore skip-delete-resources-list](.github/workflows/long-running-azure.yaml#L742)
- [Clean up cluster](.github/workflows/long-running-azure.yaml#L750)

---

## report-failure job steps

- [Create failure issue for failing long running test run](.github/workflows/long-running-azure.yaml#L770)

---

If you'd like, I can also commit this file on a new branch and open a PR for review.
