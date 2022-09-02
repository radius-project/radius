// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

type ErrFQDNOrPrefixRequired struct {
}

func (e *ErrFQDNOrPrefixRequired) Error() string {
	return "must provide either prefix or fullyQualifiedHostname if hostname is specified"
}

func (e *ErrFQDNOrPrefixRequired) Is(target error) bool {
	_, ok := target.(*ErrFQDNOrPrefixRequired)
	return ok
}

type ErrNoPublicEndpoint struct {
}

func (e *ErrNoPublicEndpoint) Error() string {
	return "no public endpoint available"
}

func (e *ErrNoPublicEndpoint) Is(target error) bool {
	_, ok := target.(*ErrNoPublicEndpoint)
	return ok
}
