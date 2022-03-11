// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"encoding/json"
	"fmt"
	"strings"
)

var kindMap = map[string]string{
	"Application":               "Application",
	"Container":                 "Container",
	"dapr.io.PubSubTopic":       "DaprIOPubSubTopic",
	"dapr.io.StateStore":        "DaprIOStateStore",
	"dapr.io.SecretStore":       "DaprIOSecretStore",
	"dapr.io.InvokeHttpRoute":   "DaprIOInvokeHttpRoute",
	"mongo.com.MongoDatabase":   "MongoDatabase",
	"rabbitmq.com.MessageQueue": "RabbitMQMessageQueue",
	"redislabs.com.RedisCache":  "RedisCache",
	"microsoft.com.SQLDatabase": "MicrosoftComSQLDatabase",
	"HttpRoute":                 "HttpRoute",
	"GrpcRoute":                 "GrpcRoute",
	"Gateway":                   "Gateway",
	"Extender":                  "Extender",
}

// TODO this should be removed and instead we should use the CR definitions to know about the arm mapping

func GetKindFromArmType(armType string) (string, bool) {
	caseInsensitive := map[string]string{}
	for k, v := range kindMap {
		k := strings.ToLower(k)
		caseInsensitive[k] = v
	}

	res, ok := caseInsensitive[strings.ToLower(armType)]
	return res, ok
}

func GetSupportedTypes() map[string]string {
	return kindMap
}

// Resource represents a (parsed) resource within an ARM template.
type Resource struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	APIVersion string    `json:"apiVersion"`
	Name       string    `json:"name"`
	DependsOn  []string  `json:"dependsOn"`
	Provider   *Provider `json:"provider,omitempty"`

	// Contains the actual payload that should be submitted (properties, kind, etc)
	// note that properties like type, name, and apiversion are present in deployment
	// templates but not in raw ARM requests. They are not in Body either.
	Body map[string]interface{} `json:"body"`
}

// Providers are extension imported in the Bicep file.
// Currently we only support 'Kubernetes'.
type Provider struct {
	Name    string `json:"provider"`
	Version string `json:"version"`
}

func (r Resource) Convert(obj interface{}) error {
	b, err := json.Marshal(r.Body)
	if err != nil {
		return fmt.Errorf("failed to convert resource to JSON: %w", err)
	}

	err = json.Unmarshal(b, obj)
	if err != nil {
		return fmt.Errorf("failed to convert resource JSON to %T: %w", obj, err)
	}

	return nil
}

// Gets the application name, resource name, and resource type for a radius resource.
// Only use on radius resource types.
func (r Resource) GetRadiusResourceParts() (applicationName string, resourceName string, resourceType string) {
	typeParts := strings.Split(r.Type, "/")
	nameParts := strings.Split(r.Name, "/")

	if len(nameParts) > 1 {
		applicationName = nameParts[1]
		resourceType = typeParts[len(typeParts)-1]
		if len(nameParts) > 2 {
			resourceName = nameParts[2]
		}
	}
	return
}
