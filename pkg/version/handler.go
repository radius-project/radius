/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package version

import (
	"encoding/json"
	"net/http"
)

// ReportVersionHandler is the http server handler to report the radius version.
func ReportVersionHandler(w http.ResponseWriter, req *http.Request) {
	info := NewVersionInfo()

	b, err := json.MarshalIndent(&info, "", "  ")

	if err != nil {
		w.WriteHeader(500)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	_, _ = w.Write(b)
}
