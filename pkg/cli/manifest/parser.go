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

package manifest

import (
	"bytes"
	"os"
	"reflect"
	"strings"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
	yaml "github.com/goccy/go-yaml"
)

// ReadFile reads a resource provider manifest from a file.
func ReadFile(filePath string) (*ResourceProvider, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return ReadBytes(data)
}

// ReadBytes reads a resource provider manifest from a byte slice.
func ReadBytes(data []byte) (*ResourceProvider, error) {
	decoder := yaml.NewDecoder(
		bytes.NewReader(data),

		// Fail on unknown fields
		// Prevent duplicate fields
		yaml.Strict(),

		// Validate fields using "validate" tags
		yaml.Validator(createValidator()))

	result := ResourceProvider{}
	err := decoder.Decode(&result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func createValidator() yaml.StructValidator {
	// This is the boilerplate required to create a validator that will support struct tags
	// like `validate:"required"` AND provide reasonable error messages.
	//
	// The yaml package has some limitations that make this a bit more complicated than it should be.
	// In particular, the yaml package doesn't call the translate function for the error messages.
	//
	// The validate package is intended to work with a translator. The defaults are not very good.
	//
	// See: https://github.com/goccy/go-yaml/blob/master/decode.go#L1409
	// for the relevant code.
	en := en.New()
	universal := ut.New(en, en)
	translator, _ := universal.GetTranslator("en")

	v := validator.New(validator.WithRequiredStructEnabled())

	// Note: we're silently ignoring errors here because we know it will never fail.
	// There will only be errors if there are duplicates. Since we're setting everything
	// up we know there are no duplicates.
	_ = en_translations.RegisterDefaultTranslations(v, translator)

	// Add validation support for our custom formats.
	_ = v.RegisterValidation("resourceProviderNamespace", resourceProviderNamespace)
	_ = v.RegisterTranslation("resourceProviderNamespace", translator, func(ut ut.Translator) error {
		return ut.Add("resourceProviderNamespace", resourceProviderNamespaceMessage, true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("resourceProviderNamespace", fe.Field())
		return t
	})

	_ = v.RegisterValidation("resourceType", validateResourceType)
	_ = v.RegisterTranslation("resourceType", translator, func(ut ut.Translator) error {
		return ut.Add("resourceType", resourceTypeMessage, true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("resourceType", fe.Field())
		return t
	})

	_ = v.RegisterValidation("apiVersion", validateAPIVersion)
	_ = v.RegisterTranslation("apiVersion", translator, func(ut ut.Translator) error {
		return ut.Add("apiVersion", apiVersionMessage, true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("apiVersion", fe.Field())
		return t
	})

	_ = v.RegisterValidation("capability", validateCapability)
	_ = v.RegisterTranslation("capability", translator, func(ut ut.Translator) error {
		return ut.Add("capability", capabilityMessage, true)
	}, func(ut ut.Translator, fe validator.FieldError) string {
		t, _ := ut.T("capability", fe.Field())
		return t
	})

	// Use the `yaml` tag for field names
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("yaml"), ",", 2)[0]
		if name == "-" {
			return fld.Name
		}
		return name
	})

	return &errorTranslator{inner: v, translator: translator}
}

// errorTranslator is a wrapper around the validator.Validate type that will
// translate the error messages into human-readable form.
type errorTranslator struct {
	inner      *validator.Validate
	translator ut.Translator
}

// Struct implements yaml.StructValidator.
func (m *errorTranslator) Struct(value any) error {
	// Call the inner validator... if we get back any errors then translate them.
	err := m.inner.Struct(value)
	if err == nil {
		return nil
	}

	// This should always match, but just in case...
	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return err
	}

	result := translatedErrors{}
	for _, fieldError := range validationErrors {
		result = append(result, translatedError{
			inner:   fieldError,
			message: fieldError.Translate(m.translator),
		})
	}

	return result
}

// Note: translatedErrors MUST follow this pattern...
//
// - It must by a slice of <something>
// - That <something> must implement the yaml.FieldError interface
// - It must implement error
type translatedErrors []translatedError

// Error implements error.
func (te translatedErrors) Error() string {
	// This should never happen, but just in case...
	if len(te) == 0 {
		return "No errors"
	}

	builder := strings.Builder{}
	for _, e := range te {
		builder.Write([]byte(e.message))
	}

	return builder.String()
}

var _ yaml.FieldError = (*translatedError)(nil)

type translatedError struct {
	message string
	inner   validator.FieldError
}

// StructField implements yaml.FieldError.
func (te translatedError) StructField() string {
	return te.inner.StructField()
}

// Error implements error.
func (te translatedError) Error() string {
	return te.message
}
