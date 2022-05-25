// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
