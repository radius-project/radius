// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import "github.com/project-radius/radius/pkg/corerp/datamodel"

// SecretObjects wraps the different secret objects to be configured on the SecretProvider class
type SecretObjects struct {
	secrets      map[string]datamodel.SecretObjectProperties
	certificates map[string]datamodel.CertificateObjectProperties
	keys         map[string]datamodel.KeyObjectProperties
}

type objectValues struct {
	alias    string
	version  string
	encoding string
	format   string
}
