// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package jobqueue

type (
	// EnqueueOptions applies an option to Enqueue().
	EnqueueOptions interface {
		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}

	// DequeueOptions applies an option to Dequeue().
	DequeueOptions interface {
		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}
)
