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

package archive

import (
	"testing"

	"github.com/radius-project/radius/pkg/statearchive"
	"go.uber.org/mock/gomock"
)

// newTestArchive injects a session directory for testing the adapter's JSON I/O.
// Durable backend behavior is covered by the OCI archive tests.
func newTestArchive(t *testing.T) *statearchive.MockArchive {
	t.Helper()
	ctrl := gomock.NewController(t)
	session := statearchive.NewMockSession(ctrl)
	session.EXPECT().Path().Return(t.TempDir()).AnyTimes()
	session.EXPECT().Commit(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	session.EXPECT().Close(gomock.Any()).AnyTimes()

	archive := statearchive.NewMockArchive(ctrl)
	archive.EXPECT().Open(gomock.Any(), DefaultGraphArchive).Return(session, nil).AnyTimes()
	return archive
}
