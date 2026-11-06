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
	"encoding/json"
	"maps"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func schema(t *testing.T) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/aws-medialive-channel.json")
	require.NoError(t, err)
	return string(b)
}

// fakes maps each handler the suite makes to its fake API.
type fakes struct {
	mu  sync.Mutex
	api map[native.Handler]*fakeAPI
}

func (f *fakes) add(h native.Handler, api *fakeAPI) native.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api == nil {
		f.api = map[native.Handler]*fakeAPI{}
	}
	f.api[h] = api
	return h
}

func (f *fakes) of(h native.Handler) *fakeAPI {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.api[h]
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

// object returns the JSON s as a value, so test templates read like the Bicep or JSON a user writes.
func object(t *testing.T, s string) any {
	t.Helper()
	var out any
	require.NoError(t, json.Unmarshal([]byte(s), &out))
	return out
}

// outputGroups returns an archive output group and a MediaPackage group, and, when key is set, an encrypted HLS
// group with key and the Akamai salt. The MediaPackage output's settings and the HLS group's M3u8Settings are empty
// objects, which select a kind of settings.
func outputGroups(t *testing.T, key string, salt string) any {
	groups := `[{
		"Name": "archive",
		"OutputGroupSettings": {"ArchiveGroupSettings": {"Destination": {"DestinationRefId": "archive"}}},
		"Outputs": [{
			"OutputName": "archive-out",
			"VideoDescriptionName": "video",
			"AudioDescriptionNames": ["audio"],
			"OutputSettings": {"ArchiveOutputSettings": {"NameModifier": "-1", "ContainerSettings": {"M2tsSettings": {"AudioBufferModel": "ATSC"}}}}
		}]
	}, {
		"Name": "mediapackage",
		"OutputGroupSettings": {"MediaPackageGroupSettings": {"Destination": {"DestinationRefId": "mp"}}},
		"Outputs": [{
			"OutputName": "mp-out",
			"VideoDescriptionName": "video",
			"AudioDescriptionNames": ["audio"],
			"OutputSettings": {"MediaPackageOutputSettings": {}}
		}]
	}`
	if key != "" {
		groups += `, {
		"Name": "hls",
		"OutputGroupSettings": {"HlsGroupSettings": {
			"Destination": {"DestinationRefId": "hls"},
			"EncryptionType": "AES128",
			"KeyProviderSettings": {"StaticKeySettings": {"StaticKeyValue": "` + key + `", "KeyProviderServer": {"Uri": "https://keys.example.com/key"}}},
			"HlsCdnSettings": {"HlsAkamaiSettings": {"Salt": "` + salt + `", "Token": "token-name", "HttpTransferMode": "CHUNKED"}}
		}},
		"Outputs": [{
			"OutputName": "hls-out",
			"VideoDescriptionName": "video",
			"AudioDescriptionNames": ["audio"],
			"OutputSettings": {"HlsOutputSettings": {"NameModifier": "_720p", "HlsSettings": {"StandardHlsSettings": {"M3u8Settings": {}}}}}
		}]
	}`
	}
	return object(t, groups+"]")
}

// channel returns a template for a small channel, with output groups.
func channel(t *testing.T, groups any) map[string]any {
	return map[string]any{
		"Name":                  "app-channel",
		"ChannelClass":          "SINGLE_PIPELINE",
		"RoleArn":               "arn:aws:iam::123456789012:role/medialive",
		"LogLevel":              "INFO",
		"ChannelSecurityGroups": []any{"1111111", "2222222"},
		"InputSpecification":    map[string]any{"Codec": "AVC", "Resolution": "HD", "MaximumBitrate": "MAX_10_MBPS"},
		"InputAttachments":      object(t, `[{"InputId": "7654321", "InputAttachmentName": "main"}]`),
		"Destinations": object(t, `[
			{"Id": "archive", "Settings": [{"Url": "s3ssl://app-media/archive"}]},
			{"Id": "mp", "MediaPackageSettings": [{"ChannelId": "app-mediapackage"}]},
			{"Id": "hls", "Settings": [{"Url": "https://p-ep.akamaientrypoint.net/cmaf/live/1/out", "Username": "akamai", "PasswordParam": "/medialive/akamai"}]}
		]`),
		"Vpc": map[string]any{"SubnetIds": []any{"subnet-1", "subnet-2"}, "SecurityGroupIds": []any{"sg-1"}},
		"EncoderSettings": map[string]any{
			"TimecodeConfig":    map[string]any{"Source": "SYSTEMCLOCK"},
			"VideoDescriptions": object(t, `[{"Name": "video", "Width": 1280, "Height": 720, "CodecSettings": {"H264Settings": {"Bitrate": 3000000, "RateControlMode": "CBR"}}}]`),
			"AudioDescriptions": object(t, `[{"Name": "audio", "AudioSelectorName": "default"}]`),
			"OutputGroups":      groups,
		},
		"Tags": map[string]any{"team": "a"},
	}
}

// The channel's secrets, an HLS encryption key and an Akamai salt, which DescribeChannel returns.
const (
	staticKey  = "0123456789abcdef0123456789abcdef"
	akamaiSalt = "akamai-salt-one"
)

var writeOnly = []string{
	"EncoderSettings/OutputGroups/*/OutputGroupSettings/HlsGroupSettings/KeyProviderSettings/StaticKeySettings/StaticKeyValue",
	"EncoderSettings/OutputGroups/*/OutputGroupSettings/HlsGroupSettings/HlsCdnSettings/HlsAkamaiSettings/Salt",
	"DryRun",
	"Vpc/PublicAddressAllocationIds",
}

func Test_Suite_Channel(t *testing.T) {
	var fs fakes
	properties := channel(t, outputGroups(t, staticKey, akamaiSalt))
	// AWS fills in defaults inside the items of lists. The output groups hold the secrets, which AWS's value can't
	// show, so a default there would count as a change (see the report on array items); video descriptions don't.
	defaulted := object(t, `[{"Name": "video", "Width": 1280, "Height": 720, "CodecSettings": {"H264Settings": {"Bitrate": 3000000, "RateControlMode": "CBR", "GopSize": 90}}}]`)
	nativetest.Run(t, nativetest.Suite{
		TypeName: channelType,
		Schema:   schema(t),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewChannel(api), api)
		},
		IdentifierProperty: "Id",
		WriteOnly:          writeOnly,
		Properties:         properties,
		Changed:            with(properties, map[string]any{"LogLevel": "DEBUG"}),
		ChangedWriteOnly:   channel(t, outputGroups(t, "fedcba9876543210fedcba9876543210", "akamai-salt-two")),
		Removable:          "Tags",
		CreateOnlyChange:   with(properties, map[string]any{"Vpc": map[string]any{"SubnetIds": []any{"subnet-3"}, "SecurityGroupIds": []any{"sg-1"}}}),
		OtherForm: map[string]any{
			"ChannelSecurityGroups":             []any{"2222222", "1111111"},
			"EncoderSettings/VideoDescriptions": defaulted,
		},
		// The suite compares the record's possibly applied values whole, but UCP merges objects such as Tags key by
		// key, so a tag the update removes stays possibly applied. MultiCall only adds and changes tags; Removable
		// covers removing them.
		MultiCall: with(properties, map[string]any{"LogLevel": "WARNING", "Tags": map[string]any{"team": "b", "env": "dev"}}),
		Calls:     2,
		FailCall: nativetest.FailCall(2, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
		// AWS fills in defaults inside video descriptions, so Equivalent compares them only on the template's values.
		// A field the template drops, such as Sharpness, must still be removed in AWS.
		RemovedItemField: &nativetest.RemovedItemField{
			Before: withVideo(t, properties, `[{"Name": "video", "Width": 1280, "Height": 720, "Sharpness": 75, "CodecSettings": {"H264Settings": {"Bitrate": 3000000, "RateControlMode": "CBR"}}}]`),
			After:  properties,
			Gone: func(stored map[string]any) bool {
				video := stored["EncoderSettings"].(map[string]any)["VideoDescriptions"].([]any)[0].(map[string]any)
				_, ok := video["Sharpness"]
				return !ok
			},
		},
	})
}

// withVideo returns properties with the video descriptions in the JSON videos.
func withVideo(t *testing.T, properties map[string]any, videos string) map[string]any {
	settings := maps.Clone(properties["EncoderSettings"].(map[string]any))
	settings["VideoDescriptions"] = object(t, videos)
	return with(properties, map[string]any{"EncoderSettings": settings})
}

// Test_ReadReturnsSecrets checks that DescribeChannel and Read return the secrets, as AWS does, so the suite's check
// that no response has them shows UCP removes them.
func Test_ReadReturnsSecrets(t *testing.T) {
	h, _ := newChannel(t)
	id := created(t, h, channel(t, outputGroups(t, staticKey, akamaiSalt)))
	got, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(raw), staticKey)
	require.Contains(t, string(raw), akamaiSalt)
}
