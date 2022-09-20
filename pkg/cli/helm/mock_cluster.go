// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import "errors"

var _ Interface = (*MockHelmInterface)(nil)

type MockHelmInterface struct {

}

func(mi *MockHelmInterface) CheckRadiusInstall(kubeContext string) (bool, error) {
	if kubeContext == "kind-kind" {
		return true, nil
	}
	return false, errors.New("radius control plane is not installed")
}