/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import "time"

type (
	// EnqueueOptions applies an option to Enqueue().
	EnqueueOptions interface {
		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}

	DequeueOptions interface {
		// ApplyDequeueOption applies DequeueOptions to QueueClientConfig.
		ApplyDequeueOption(QueueClientConfig) QueueClientConfig
		// A private method to prevent users implementing the
		// interface and so future additions to it will not
		// violate compatibility.
		private()
	}
)

// QueueClientConfig is a configuration for queue client APIs.
type QueueClientConfig struct {
	// DequeueIntervalDuration is the time duration between 2 successive dequeue attempts on the queue
	DequeueIntervalDuration time.Duration
}

type dequeueOptions struct {
	fn func(QueueClientConfig) QueueClientConfig
}

// ApplyDequeueOption applies the configuration to the queue client.
func (q *dequeueOptions) ApplyDequeueOption(cfg QueueClientConfig) QueueClientConfig {
	return q.fn(cfg)
}

// WithDequeueInterval sets dequeueing interval.
func WithDequeueInterval(t time.Duration) DequeueOptions {
	return &dequeueOptions{
		fn: func(cfg QueueClientConfig) QueueClientConfig {
			cfg.DequeueIntervalDuration = t
			return cfg
		},
	}
}

func (q dequeueOptions) private() {}

// NewDequeueConfig returns new queue config for StartDequeuer().
func NewDequeueConfig(opts ...DequeueOptions) QueueClientConfig {
	cfg := QueueClientConfig{}
	for _, opt := range opts {
		cfg = opt.ApplyDequeueOption(cfg)
	}
	return cfg
}
