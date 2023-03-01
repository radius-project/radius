// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metrics

var (
	// DefaultAsyncOperationMetrics holds async operation metrics definitions.
	DefaultAsyncOperationMetrics = newAsyncOperationMetrics()
)

// InitMetrics initializes metrics for Radius.
func InitMetrics() error {
	if err := DefaultAsyncOperationMetrics.Init(); err != nil {
		return err
	}

	return nil
}
