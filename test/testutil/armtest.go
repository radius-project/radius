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

package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

func GetARMTestHTTPRequestFromURL(ctx context.Context, method string, url string, body []byte) (*http.Request, error) {
	headers := map[string]string{
		"Accept":          "application/json",
		"Accept-Encoding": "gzip, deflate",
		"Accept-Language": "en-US",
		"Content-Length":  "305",
		"Content-Type":    "application/json; charset=utf-8",
	}
	req, _ := http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Add(k, v)
	}
	req.Header.Add("Referer", url)
	return req, nil
}

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
	return ctx
}
