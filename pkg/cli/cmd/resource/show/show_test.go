// // ------------------------------------------------------------
// // Copyright (c) Microsoft Corporation.
// // Licensed under the MIT License.
// // ------------------------------------------------------------

package show

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/cmd/mocks"
	"github.com/project-radius/radius/test/radcli"
)

func Test_CommandValidation(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	connectionsFactoryMock := mocks.NewMockConnectionsFactory(mockCtrl)
	configMock := mocks.NewMockConfigInterface(mockCtrl)
	radcli.SharedCommandValidation(t, NewCommand(connectionsFactoryMock, configMock))
}
