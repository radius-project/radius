// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"math/rand"
	"strings"
	"time"

	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/outputresource"
)

// mergeProperties combines properties from a resource definition and a potentially existing resource.
// This is useful for cases where deploying a resource results in storage of generated values like names.
// By merging properties, the caller gets to see those values and reuse them.
func mergeProperties(resource outputresource.OutputResource, existing *db.DeploymentResource) map[string]string {
	properties := resource.Resource.(map[string]string)
	if properties == nil {
		properties = map[string]string{}
	}

	if existing == nil {
		return properties
	}

	for k, v := range existing.Properties {
		_, ok := properties[k]
		if !ok {
			properties[k] = v
		}
	}

	return properties
}

type PasswordConditions struct {
	Lower       int
	Upper       int
	Digits      int
	SpecialChar int
}

func generatePassword(conditions *PasswordConditions) string {
	pwd := generateString(conditions.Digits, "1234567890") +
		generateString(conditions.Lower, "abcdefghijklmnopqrstuvwxyz") +
		generateString(conditions.Upper, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") +
		generateString(conditions.SpecialChar, "-")

	pwdArray := strings.Split(pwd, "")
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(pwdArray), func(i, j int) {
		pwdArray[i], pwdArray[j] = pwdArray[j], pwdArray[i]
	})
	pwd = strings.Join(pwdArray, "")

	return pwd
}

func generateString(length int, allowedCharacters string) string {
	str := ""
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	for len(str) < length {
		str += string(allowedCharacters[rnd.Intn(len(allowedCharacters))])
	}
	return str
}
