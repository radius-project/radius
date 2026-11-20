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

package chime

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region = "us-west-2"

	// testVC and otherVC are IDs of Voice Connectors the fake has, missingVC of one it doesn't have, and emergencyVC of
	// one with an emergency calling configuration.
	testVC      = "abcdefghijklmnopqrstuv"
	otherVC     = "vutsrqponmlkjihgfedcba"
	missingVC   = missingPrefix + "abcdefghijklmno"
	emergencyVC = emergencyPrefix + "abcdefghijklm"
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// withoutWait returns h, which waits with noWait.
func withoutWait[H interface{ setWait(pause) }](h H) H {
	h.setWait(noWait)
	return h
}

func (h *LoggingConfiguration) setWait(p pause)   { h.wait = p }
func (h *StreamingConfiguration) setWait(p pause) { h.wait = p }
func (h *Origination) setWait(p pause)            { h.wait = p }
func (h *Termination) setWait(p pause)            { h.wait = p }

func logging(vc string, sip bool, media bool) map[string]any {
	return map[string]any{"VoiceConnectorId": vc, "LoggingConfiguration": map[string]any{"EnableSIPLogs": sip, "EnableMediaMetricLogs": media}}
}

func streaming(vc string, settings map[string]any) map[string]any {
	return map[string]any{"VoiceConnectorId": vc, "StreamingConfiguration": settings}
}

func origination(vc string, routes ...map[string]any) map[string]any {
	list := []any{}
	for _, route := range routes {
		list = append(list, route)
	}
	return map[string]any{"VoiceConnectorId": vc, "Origination": map[string]any{"Routes": list, "Disabled": false}}
}

func route(host string, priority int, weight int) map[string]any {
	return map[string]any{"Host": host, "Port": 5060, "Protocol": "UDP", "Priority": priority, "Weight": weight}
}

func termination(vc string, settings map[string]any) map[string]any {
	return map[string]any{"VoiceConnectorId": vc, "Termination": settings}
}

func Test_Suite_LoggingConfiguration(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    loggingType,
		Schema:      readFile(t, "aws-chime-voiceconnectorloggingconfiguration.json"),
		Metadata:    readFile(t, "aws-chime-voiceconnectorloggingconfiguration.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return withoutWait(NewLoggingConfiguration(newFakeAPI(store)))
		},
		IdentifierProperty: voiceConnectorIDProperty,
		Properties:         logging(testVC, true, false),
		Identifier:         testVC,
		Changed:            logging(testVC, true, true),
		CreateOnlyChange:   logging(otherVC, true, false),
		ResetTo:            logging(testVC, false, false),
		AlwaysPresent:      true,
		API:                loggingAPI,
	})
}

func Test_Suite_StreamingConfiguration(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    streamingType,
		Schema:      readFile(t, "aws-chime-voiceconnectorstreamingconfiguration.json"),
		Metadata:    readFile(t, "aws-chime-voiceconnectorstreamingconfiguration.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return withoutWait(NewStreamingConfiguration(newFakeAPI(store)))
		},
		IdentifierProperty: voiceConnectorIDProperty,
		Properties:         streaming(testVC, map[string]any{"DataRetentionInHours": 24, "Disabled": false}),
		Identifier:         testVC,
		Changed: streaming(testVC, map[string]any{"DataRetentionInHours": 48, "Disabled": false, "StreamingNotificationTargets": []any{
			map[string]any{"NotificationTarget": "SNS"},
		}}),
		CreateOnlyChange: streaming(otherVC, map[string]any{"DataRetentionInHours": 24, "Disabled": false}),
		API:              streamingAPI,
	})
}

func Test_Suite_Origination(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName:    originationType,
		Schema:      readFile(t, "aws-chime-voiceconnectororigination.json"),
		Metadata:    readFile(t, "aws-chime-voiceconnectororigination.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return withoutWait(NewOrigination(newFakeAPI(store)))
		},
		IdentifierProperty: voiceConnectorIDProperty,
		Properties:         origination(testVC, route("sip1.example.com", 1, 1)),
		Identifier:         testVC,
		Changed:            origination(testVC, route("sip1.example.com", 1, 1), route("sip2.example.com", 2, 1)),
		CreateOnlyChange:   origination(otherVC, route("sip1.example.com", 1, 1)),
		API:                originationAPI,
	})
}

func Test_Suite_Termination(t *testing.T) {
	settings := func(regions ...any) map[string]any {
		return map[string]any{"CallingRegions": regions, "CidrAllowedList": []any{"203.0.113.0/27"}, "DefaultPhoneNumber": "+12065550100"}
	}
	changedPhone := settings("US")
	changedPhone["DefaultPhoneNumber"] = "+12065550101"
	nativetest.Run(t, nativetest.Suite{
		TypeName:    terminationType,
		Schema:      readFile(t, "aws-chime-voiceconnectortermination.json"),
		Metadata:    readFile(t, "aws-chime-voiceconnectortermination.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return withoutWait(NewTermination(newFakeAPI(store)))
		},
		IdentifierProperty: voiceConnectorIDProperty,
		WriteOnly:          []string{"Termination/DefaultPhoneNumber"},
		Properties:         termination(testVC, settings("US")),
		Identifier:         testVC,
		Changed:            termination(testVC, settings("US", "CA")),
		ChangedWriteOnly:   termination(testVC, changedPhone),
		CreateOnlyChange:   termination(otherVC, settings("US")),
		API:                terminationAPI,
	})
}
