package helm

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	containerderrors "github.com/containerd/containerd/remotes/errors"
	"github.com/stretchr/testify/assert"
)

func TestExtractHelmError(t *testing.T) {
	err := errors.New("error")
	extractedErr := extractHelmError(err)
	assert.Nil(t, extractedErr)

	err = fmt.Errorf("%w: wrapped error", errors.New("error"))
	extractedErr = extractHelmError(err)
	assert.Nil(t, extractedErr)

	err = fmt.Errorf("%w: wrapped error", containerderrors.ErrUnexpectedStatus{})
	extractedErr = extractHelmError(err)
	assert.Nil(t, extractedErr)

	err = fmt.Errorf("%w: wrapped error", containerderrors.ErrUnexpectedStatus{StatusCode: http.StatusForbidden, RequestURL: "ghcr.io/myregistry"})
	extractedErr = extractHelmError(err)
	assert.Equal(t, errors.New("recieved 403 unauthorized when downloading helm chart from the registry. you may want to perform a `docker logout ghcr.io` and re-try the command"), extractedErr)

	err = containerderrors.ErrUnexpectedStatus{StatusCode: http.StatusForbidden, RequestURL: "ghcr.io/myregistry"}
	extractedErr = extractHelmError(err)
	assert.Equal(t, errors.New("recieved 403 unauthorized when downloading helm chart from the registry. you may want to perform a `docker logout ghcr.io` and re-try the command"), extractedErr)
}
