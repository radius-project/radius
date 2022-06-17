// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import "errors"

const (
	ResourceTypeName = "Applications.Core/containers"
)

var (
	ErrOngoingAsyncOperationOnResource = errors.New("the source or target resource group is locked (e.g. move already in progress, resource group is being deleted)")
)
