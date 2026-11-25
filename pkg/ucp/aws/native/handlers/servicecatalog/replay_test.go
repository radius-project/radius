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

package servicecatalog

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_AcceptedPortfolioShare accepts a portfolio another account shared with the recording account, and
// rejects it, against AWS's recorded responses. Every property is create-only, so there is no update step.
func Test_Replay_AcceptedPortfolioShare(t *testing.T) {
	s := awsreplay.New(t, "testdata/acceptedportfolioshare.json")
	api := servicecatalog.NewFromConfig(s.Config(t.Context()))

	properties := map[string]any{
		"PortfolioId":    s.Input("RADIUS_REC_SHARED_PORTFOLIO_ID"),
		"AcceptLanguage": "en",
	}
	nativetest.Lifecycle{
		Handler:    NewAcceptedPortfolioShare(api),
		Session:    s,
		Properties: properties,
		Want:       map[string]any{"PortfolioId": properties["PortfolioId"]},
		Poll:       5 * time.Second,
	}.Run(t)
}
