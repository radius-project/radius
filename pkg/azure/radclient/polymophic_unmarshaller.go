// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package radclient

import "encoding/json"

// UnmarshalComponentTraitClassification parses a JSON message into a
// ComponentTraitClassification in a polymorphic way.
func UnmarshalComponentTraitClassification(rawMsg json.RawMessage) (ComponentTraitClassification, error) {
	return unmarshalComponentTraitClassification(rawMsg)
}
