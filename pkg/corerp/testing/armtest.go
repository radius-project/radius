// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package testing

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
)

func GetARMTestHTTPRequest(ctx context.Context, method string, headerFixtureJSONFile string, body any) (*http.Request, error) {
	jsonData, err := os.ReadFile("./testdata/" + headerFixtureJSONFile)
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
	armctx, _ := v1.FromARMRequest(req, "", "West US")
	ctx = v1.WithARMRequestContext(ctx, armctx)
	ctx = hostoptions.WithContext(ctx, &hostoptions.ProviderConfig{
		Env: hostoptions.EnvironmentOptions{RoleLocation: "West US"},
	})
	return ctx
}
