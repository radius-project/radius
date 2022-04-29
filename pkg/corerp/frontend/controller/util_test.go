package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

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
