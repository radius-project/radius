// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package utils

import (
	"encoding/json"
	"testing"

	"github.com/Azure/radius/pkg/radclient"
	"github.com/google/go-cmp/cmp"
)

func stringP(s string) *string {
	return &s
}

func TestGenerateErrorMessage(t *testing.T) {
	type Info struct {
		Code           string                          `json:"code"`
		Target         string                          `json:"target"`
		Message        string                          `json:"message"`
		Details        []radclient.ErrorDetail         `json:"details"`
		AdditionalInfo []radclient.ErrorAdditionalInfo `json:"additionalInfo"`
	}
	for _, tc := range []struct {
		name       string
		innerError radclient.ErrorDetail
		want       Info
	}{{
		name: "empty inner error",
	}, {
		name: "has message",
		innerError: radclient.ErrorDetail{
			Message: stringP("foo"),
		},
		want: Info{
			Message: "foo",
		},
	}, {
		name: "has target",
		innerError: radclient.ErrorDetail{
			Target: stringP("red dot in red circle"),
		},
		want: Info{
			Target: "red dot in red circle",
		},
	}, {
		name: "has details",
		innerError: radclient.ErrorDetail{
			Details: &[]radclient.ErrorDetail{{
				Message: stringP("to be"),
			}, {
				Message: stringP("or not to be"),
			}},
		},
		want: Info{
			Details: []radclient.ErrorDetail{{
				Message: stringP("to be"),
			}, {
				Message: stringP("or not to be"),
			}},
		},
	}, {
		name: "has additional info",
		innerError: radclient.ErrorDetail{
			AdditionalInfo: &[]radclient.ErrorAdditionalInfo{{
				Type: stringP("info type"),
				Info: map[string]string{
					"key": "value",
				},
			}},
		},
		want: Info{
			AdditionalInfo: []radclient.ErrorAdditionalInfo{{
				Type: stringP("info type"),
				Info: map[string]interface{}{
					"key": "value",
				},
			}},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got := Info{}
			msg := GenerateErrorMessage(radclient.ErrorResponse{
				InnerError: &tc.innerError,
			})
			err := json.Unmarshal([]byte(msg), &got)
			if err != nil {
				t.Error("Expect result to be valid JSON, saw: ", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("GenerateErrorMessage() mismatch (-want +got):\n%s", diff)
			}
		})
	}
	t.Run("no inner error", func(t *testing.T) {
		want := "missing error info"
		got := GenerateErrorMessage(radclient.ErrorResponse{})
		if want != got {
			t.Errorf("GenerateErrorMessage() mismatch: want=%q, got=%q", want, got)
		}
	})
}
