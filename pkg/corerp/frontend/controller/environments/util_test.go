// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	v20220315privatepreview "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
)

func GetTestHTTPRequest(ctx context.Context, method string, headerFileName string, body interface{}) (*http.Request, error) {
	jsonData, err := ioutil.ReadFile("./testdata/" + headerFileName)
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

func GetTestRequestContext(req *http.Request) context.Context {
	ctx := context.Background()
	armctx, _ := servicecontext.FromARMRequest(req, "")
	ctx = servicecontext.WithARMRequestContext(ctx, armctx)
	ctx = hostoptions.WithContext(ctx, &hostoptions.ProviderConfig{
		CloudEnv: hostoptions.CloudEnvironmentOptions{RoleLocation: "West US"},
	})
	return ctx
}

func GetJsonData(filename string) ([]byte, error) {
	return ioutil.ReadFile("./testdata/" + filename)
}

func GetTestModels20220315privatepreview() (*v20220315privatepreview.EnvironmentResource, *datamodel.Environment, *v20220315privatepreview.EnvironmentResource) {
	rawInput, _ := GetJsonData("environment20220315privatepreview_input.json")
	envInput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawInput, envInput)

	rawDataModel, _ := GetJsonData("environment20220315privatepreview_datamodel.json")
	envDataModel := &datamodel.Environment{}
	_ = json.Unmarshal(rawDataModel, envDataModel)

	rawExpectedOutput, _ := GetJsonData("environment20220315privatepreview_output.json")
	expectedOutput := &v20220315privatepreview.EnvironmentResource{}
	_ = json.Unmarshal(rawExpectedOutput, expectedOutput)

	return envInput, envDataModel, expectedOutput
}
