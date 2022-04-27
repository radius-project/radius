// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testing

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
)

func GetARMTestHTTPRequest(ctx context.Context, method string, headerFixtureJSONFile string, body interface{}) (*http.Request, error) {
	jsonData, err := ioutil.ReadFile("./testdata/" + headerFixtureJSONFile)
	if err != nil {
		return nil, err
	}

	parsed := map[string]string{}
	if err = json.Unmarshal(jsonData, &parsed); err != nil {
		return nil, err
	}

	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}

	req, _ := http.NewRequestWithContext(ctx, method, parsed["Referer"], bytes.NewBuffer(raw))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range parsed {
		req.Header.Add(k, v)
	}
	return req, nil
}

func ARMTestContextFromRequest(req *http.Request) context.Context {
	ctx := context.Background()
	armctx, _ := servicecontext.FromARMRequest(req, "")
	ctx = servicecontext.WithARMRequestContext(ctx, armctx)
	ctx = hostoptions.WithContext(ctx, &hostoptions.ProviderConfig{
		CloudEnv: hostoptions.CloudEnvironmentOptions{RoleLocation: "West US"},
	})
	return ctx
}
