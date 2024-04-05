package helm

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnwrapAll(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	err3 := errors.New("error 3")

	err := UnwrapAll(err1)
	assert.Equal(t, err1, err)

	err = UnwrapAll(fmt.Errorf("%w: wrapped error 2", err2))
	assert.Equal(t, err2, err)

	err = UnwrapAll(fmt.Errorf("%w: wrapped error 2", fmt.Errorf("%w: wrapped error 3", err3)))
	assert.Equal(t, err3, err)
}
