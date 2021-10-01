// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

const (
	ResourceType = "mongodb.com.MongoDBComponent"
)

type MongoDBComponentProperties struct {
	Managed  bool   `json:"managed"`
	Resource string `json:"resource"`
}
