package header

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
)

var tag string = uuid.New().String()

func TestValidate_IfMatch_TagsMatch(t *testing.T) {
	armRequestContext := servicecontext.ARMRequestContextFromContext(
		servicecontext.WithARMRequestContext(
			context.Background(), &servicecontext.ARMRequestContext{
				IfMatch: tag,
			}))

	err := Validate(*armRequestContext, tag)

	if err != nil {
		t.Errorf("Error thrown even though the tags match")
	}
}

func TestValidate_IfMatch_TagsDoNotMatch(t *testing.T) {
	armRequestContext := servicecontext.ARMRequestContextFromContext(
		servicecontext.WithARMRequestContext(
			context.Background(), &servicecontext.ARMRequestContext{
				IfMatch: tag,
			}))

	err := Validate(*armRequestContext, uuid.New().String())

	if err.Error() != "resource tags do not match" {
		t.Errorf("Error message is wrong")
	}
}

func TestValidate_IfMatch_Wildcard(t *testing.T) {
	armRequestContext := servicecontext.ARMRequestContextFromContext(
		servicecontext.WithARMRequestContext(
			context.Background(), &servicecontext.ARMRequestContext{
				IfMatch: "*",
			}))

	err := Validate(*armRequestContext, tag)

	if err != nil {
		t.Errorf("Error is thrown even though wildcard is used in if-match header")
	}
}

func TestValidate_IfNoneMatch_Wildcard(t *testing.T) {
	armRequestContext := servicecontext.ARMRequestContextFromContext(
		servicecontext.WithARMRequestContext(
			context.Background(), &servicecontext.ARMRequestContext{
				IfNoneMatch: "*",
			}))

	err := Validate(*armRequestContext, tag)

	if err.Error() != "resource already exists" {
		t.Errorf("Error message is wrong")
	}
}

func TestValidate_HappyPath_NoHeader(t *testing.T) {
	armRequestContext := servicecontext.ARMRequestContextFromContext(
		servicecontext.WithARMRequestContext(
			context.Background(), &servicecontext.ARMRequestContext{}))

	err := Validate(*armRequestContext, tag)

	if err != nil {
		t.Errorf("Error must be nil if neither If-Match nor If-None-Match is provided")
	}
}
