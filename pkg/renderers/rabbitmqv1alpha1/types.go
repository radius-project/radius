// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha1

const Kind = "rabbitmq.com/MessageQueue@v1alpha1"
const ResourceType = "rabbitmq.com.MessageQueueComponent"

const QueueNameKey = "queue"

// RabbitMQComponent is the definition of the service bus queue component
type RabbitMQComponent struct {
	Name     string                   `json:"name"`
	Kind     string                   `json:"kind"`
	Config   RabbitMQConfig           `json:"config,omitempty"`
	Run      map[string]interface{}   `json:"run,omitempty"`
	Uses     []map[string]interface{} `json:"uses,omitempty"`
	Bindings []map[string]interface{} `json:"bindings,omitempty"`
	Traits   []map[string]interface{} `json:"traits,omitempty"`
}

// RabbitMQConfig is the defintion of the config section
type RabbitMQConfig struct {
	Managed  bool   `json:"managed"`
	Queue    string `json:"queue"`
	Resource string `json:"resource"`
}
