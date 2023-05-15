// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package common

// LongDescriptionBlurb is a blurb that's included in all of the command descriptions for 'provider'.
// The newlines are intentional, don't make changes without looking at the formatting.
const LongDescriptionBlurb = `

Radius cloud providers enable Radius environments to deploy and integrate with cloud resources (Azure, AWS).
The Radius control-plane stores credentials for use when accessing cloud resources.

Cloud providers are configured per-Radius-installation. Configuration commands will use the current workspace
or the workspace specified by '--workspace' to configure Radius. Modifications to cloud provider configuration
or credentials will affect all Radius environments and applications of the affected installation.`
