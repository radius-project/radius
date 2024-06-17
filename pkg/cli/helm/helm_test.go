package helm

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	containerderrors "github.com/containerd/containerd/remotes/errors"
	"github.com/stretchr/testify/assert"
)

func Test_isHelm403Error(t *testing.T) {
	var err error
	var result bool

	err = errors.New("error")
	result = isHelm403Error(err)
	assert.False(t, result)

	err = fmt.Errorf("%w: wrapped error", errors.New("error"))
	result = isHelm403Error(err)
	assert.False(t, result)

	err = fmt.Errorf("%w: wrapped error", containerderrors.ErrUnexpectedStatus{})
	result = isHelm403Error(err)
	assert.False(t, result)

	err = fmt.Errorf("%w: wrapped error", containerderrors.ErrUnexpectedStatus{StatusCode: http.StatusForbidden, RequestURL: "ghcr.io/myregistry"})
	result = isHelm403Error(err)
	assert.True(t, result)

	err = containerderrors.ErrUnexpectedStatus{StatusCode: http.StatusForbidden, RequestURL: "ghcr.io/myregistry"}
	result = isHelm403Error(err)
	assert.True(t, result)

	err = containerderrors.ErrUnexpectedStatus{StatusCode: http.StatusUnauthorized, RequestURL: "ghcr.io/myregistry"}
	result = isHelm403Error(err)
	assert.False(t, result)
}
