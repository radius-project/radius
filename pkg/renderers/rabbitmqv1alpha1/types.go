// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha1

const ResourceType = "rabbitmq.com.MessageQueueComponent"

const QueueNameKey = "queue"

type Properties struct {
	Managed  bool   `json:"managed"`
	Queue    string `json:"queue"`
	Resource string `json:"resource"`
}
