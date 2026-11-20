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

package elbv2

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_ListenerCertificate adds a certificate to an HTTPS listener, replaces it with another, and removes it,
// against AWS's recorded responses. The listener's default certificate stays.
func Test_Replay_ListenerCertificate(t *testing.T) {
	s := awsreplay.New(t, "testdata/listener-certificate.json")
	api := elasticloadbalancingv2.NewFromConfig(s.Config(t.Context()))
	listenerArn := s.Input("RADIUS_REC_HTTPS_LISTENER_ARN")
	first := s.Input("RADIUS_REC_CERTIFICATE_ARN")
	second := s.Input("RADIUS_REC_CERTIFICATE_ARN_2")

	nativetest.Lifecycle{
		Handler:    NewListenerCertificate(api),
		Session:    s,
		Properties: properties(listenerArn, first),
		Update:     properties(listenerArn, second),
	}.Run(t)
}
