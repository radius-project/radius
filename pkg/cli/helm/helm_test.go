package helm

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	containerderrors "github.com/containerd/containerd/remotes/errors"
	"github.com/stretchr/testify/assert"
)

func TestUnwrapAll(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	err3 := errors.New("error 3")

	err := unwrapAll(err1)
	assert.Equal(t, err1, err)

	err = unwrapAll(fmt.Errorf("%w: wrapped error 2", err2))
	assert.Equal(t, err2, err)

	err = unwrapAll(fmt.Errorf("%w: wrapped error 2", fmt.Errorf("%w: wrapped error 3", err3)))
	assert.Equal(t, err3, err)
}

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
