package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadJSONBody_ExtractJSONContent(t *testing.T) {
	// arrange
	content, _ := json.Marshal(map[string]string{
		"id":   "fakeID",
		"type": "fakeType",
	})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, "http://github.com", bytes.NewBuffer(content))
	req.Header.Set("Content-Type", "application/json")
	// act
	parsed, err := ReadJSONBody(req)
	// assert
	require.NoError(t, err)
	require.Equal(t, string(content), string(parsed))
}

func TestReadJSONBody_NonJSONContent(t *testing.T) {
	// arrange
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, "http://github.com", bytes.NewBufferString("fakebody"))
	req.Header.Set("Content-Type", "plain/text")
	// act
	_, err := ReadJSONBody(req)
	// assert
	require.ErrorIs(t, err, ErrUnsupportedContentType)
}
