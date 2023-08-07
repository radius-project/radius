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

package handlers

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/azure/armauth"
)

const (
	FileShareNameKey = "fileshare"
	FileShareIDKey   = "fileshareid"
)

// # Function Explanation
//
// NewAzureFileShareHandler creates a new instance of azureFileShareHandler with the given arm config.
func NewAzureFileShareHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureFileShareHandler{arm: arm}
}

type azureFileShareHandler struct {
	arm *armauth.ArmConfig
}

// # Function Explanation
//
// Put validates the required properties for the resource and creates/modifies the resource using the ARMHandler.
func (handler *azureFileShareHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties, ok := options.Resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	// This assertion is important so we don't start creating/modifying a resource
	err := ValidateResourceIDsForResource(properties, FileShareStorageAccountIDKey, FileShareIDKey, FileShareNameKey)
	if err != nil {
		return nil, err
	}

	armhandler := NewARMHandler(handler.arm)
	properties, err = armhandler.Put(ctx, options)
	if err != nil {
		return nil, err
	}
	return properties, nil
}

// # Function Explanation
//
// No-op. Just returns nil.
func (handler *azureFileShareHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}
