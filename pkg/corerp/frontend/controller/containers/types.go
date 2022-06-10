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
	ErrOngoingAsyncOperationOnResource = errors.New("there is an ongoing async operation on the resource")
)
