// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secrets

type Interface interface {
	CreateSecrets(name string)
	DeleteSecrets(name string)
	GetSecrets(name string)
	ListSecrets()
}