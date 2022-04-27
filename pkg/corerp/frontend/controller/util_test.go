package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/stretchr/testify/require"
)

func TestReadJSONBody(t *testing.T) {
	content, _ := json.Marshal(map[string]string{
		"id":   "fakeID",
		"type": "fakeType",
	})

	contentTypeTests := []struct {
		contentType string
		body        []byte
		err         error
	}{
		{"application/json", content, nil},
		{"application/json; charset=utf8", content, nil},
		{"application/json;    charset=utf8", content, nil},
		{"Application/Json;    charset=utf8    ", content, nil},
		{"plain/text", content, ErrUnsupportedContentType},
	}

	for _, tc := range contentTypeTests {
		t.Run(tc.contentType, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, "http://github.com", bytes.NewBuffer(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			// act
			parsed, err := ReadJSONBody(req)
			// assert
			if tc.err != nil {
				require.ErrorIs(t, tc.err, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, string(tc.body), string(parsed))
			}
		})
	}
}

var tag string = uuid.New().String()

func TestValidateEtag_IfMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ifMatchEtag  string
		etagProvided string
		shouldFail   bool
	}{
		{"", "existingEtag", false},
		{"", "", false},
		{tag, tag, false},
		{tag, uuid.New().String(), true},
		{"*", "", true},
		{"*", tag, false},
	}

	for _, tt := range cases {
		t.Run(tt.ifMatchEtag, func(t *testing.T) {
			armRequestContext := servicecontext.ARMRequestContextFromContext(
				servicecontext.WithARMRequestContext(
					context.Background(), &servicecontext.ARMRequestContext{
						IfMatch: tt.ifMatchEtag,
					}))
			result := ValidateETag(*armRequestContext, tt.etagProvided)
			if !tt.shouldFail && result != nil {
				t.Errorf("test failed even though it should not have")
				t.Logf(result.Error())
			}
		})
	}
}

func TestValidateEtag_IfNoneMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ifNoneMatchEtag string
		etagProvided    string
		shouldFail      bool
	}{
		{"", "", false},
		{"", tag, false},
		{"*", "", false},
		{"*", tag, true},
	}

	for _, tt := range cases {
		t.Run(tt.ifNoneMatchEtag, func(t *testing.T) {
			armRequestContext := servicecontext.ARMRequestContextFromContext(
				servicecontext.WithARMRequestContext(
					context.Background(), &servicecontext.ARMRequestContext{
						IfNoneMatch: tt.ifNoneMatchEtag,
					}))
			result := ValidateETag(*armRequestContext, tt.etagProvided)
			if !tt.shouldFail && result != nil {
				t.Errorf("test failed even though it should not have")
				t.Logf(result.Error())
			}
		})
	}
}
