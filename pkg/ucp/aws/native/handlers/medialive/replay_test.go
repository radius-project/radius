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

package medialive

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/medialive"

	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_Channel creates a single-pipeline channel with one encrypted HLS output, changes its log level, and
// deletes it, against AWS's recorded responses. The channel is never started, so it encodes nothing. Its HLS key and
// Akamai salt are scrubbed from the cassette, though DescribeChannel returns them.
func Test_Replay_Channel(t *testing.T) {
	s := awsreplay.New(t, "testdata/channel.json")
	api := medialive.NewFromConfig(s.Config(t.Context()))
	run := s.Input("RADIUS_REC_RUN")
	key := "52616469757352656300000000000000"
	salt := "radius-rec-salt-" + run
	s.Scrub(key, salt)

	encoder := object(t, `{
		"TimecodeConfig": {"Source": "SYSTEMCLOCK"},
		"VideoDescriptions": [{"Name": "video", "Width": 640, "Height": 360,
			"CodecSettings": {"H264Settings": {"RateControlMode": "CBR", "Bitrate": 500000, "FramerateControl": "SPECIFIED", "FramerateNumerator": 30, "FramerateDenominator": 1}}}],
		"AudioDescriptions": [{"Name": "audio", "AudioSelectorName": "default", "CodecSettings": {"AacSettings": {"Bitrate": 64000}}}],
		"OutputGroups": [{
			"Name": "hls",
			"OutputGroupSettings": {"HlsGroupSettings": {
				"Destination": {"DestinationRefId": "hls"},
				"EncryptionType": "AES128",
				"KeyProviderSettings": {"StaticKeySettings": {"StaticKeyValue": "`+key+`", "KeyProviderServer": {"Uri": "https://example.com/radius/key"}}},
				"HlsCdnSettings": {"HlsAkamaiSettings": {"Salt": "`+salt+`", "Token": "radius-rec"}}
			}},
			"Outputs": [{"OutputName": "360p", "VideoDescriptionName": "video", "AudioDescriptionNames": ["audio"],
				"OutputSettings": {"HlsOutputSettings": {"NameModifier": "_360p", "HlsSettings": {"StandardHlsSettings": {"M3u8Settings": {}}}}}}]
		}]
	}`)
	properties := map[string]any{
		"Name":               "radius-rec-" + run,
		"ChannelClass":       "SINGLE_PIPELINE",
		"RoleArn":            s.Input("RADIUS_REC_MEDIALIVE_ROLE_ARN"),
		"LogLevel":           "DISABLED",
		"InputSpecification": map[string]any{"Codec": "AVC", "Resolution": "SD", "MaximumBitrate": "MAX_10_MBPS"},
		"InputAttachments":   []any{map[string]any{"InputId": s.Input("RADIUS_REC_MEDIALIVE_INPUT_ID"), "InputAttachmentName": "main"}},
		"Destinations":       []any{map[string]any{"Id": "hls", "Settings": []any{map[string]any{"Url": "https://radius-rec.example.com/live/" + run + "/index"}}}},
		"EncoderSettings":    encoder,
		"Tags":               map[string]any{"radius-recording": run},
	}
	want := map[string]any{
		"Name":         properties["Name"],
		"ChannelClass": "SINGLE_PIPELINE",
		"RoleArn":      properties["RoleArn"],
		"LogLevel":     "DISABLED",
		"Tags":         properties["Tags"],
	}
	nativetest.Lifecycle{
		Handler:     NewChannel(api),
		Session:     s,
		Properties:  properties,
		Want:        want,
		Update:      with(properties, map[string]any{"LogLevel": "ERROR"}),
		WantUpdated: with(want, map[string]any{"LogLevel": "ERROR"}),
		Poll:        10 * time.Second,
	}.Run(t)
}
