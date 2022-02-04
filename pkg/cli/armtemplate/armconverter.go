// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"strings"
)

var kindMap = map[string]string{
	"Application":               "Application",
	"Container":                 "Container",
	"dapr.io.PubSubTopic":       "DaprIOPubSubTopic",
	"dapr.io.StateStore":        "DaprIOStateStore",
	"dapr.io.InvokeHttpRoute":   "DaprIOInvokeHttpRoute",
	"mongo.com.MongoDatabase":   "MongoDatabase",
	"rabbitmq.com.MessageQueue": "RabbitMQMessageQueue",
	"redislabs.com.RedisCache":  "RedisCache",
	"microsoft.com.SQLDatabase": "MicrosoftComSQLDatabase",
	"HttpRoute":                 "HttpRoute",
	"GrpcRoute":                 "GrpcRoute",
	"Gateway":                   "Gateway",
	"Generic":                   "Generic",
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
