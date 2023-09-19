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

package v1

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	utcLayoutJSON = `"2006-01-02T15:04:05.999999999"`
	utcLayout     = "2006-01-02T15:04:05.999999999"
	rfc3339JSON   = `"` + time.RFC3339Nano + `"`
)

// Azure reports time in UTC but it doesn't include the 'Z' time zone suffix in some cases.
var tzOffsetRegex = regexp.MustCompile(`(Z|z|\+|-)(\d+:\d+)*"*$`)

type timeRFC3339 time.Time

// MarshalJSON is a method that converts the timeRFC3339 object into a JSON formatted byte array.
// It returns the JSON formatted byte array and an error if any occurred during the marshalling process.
func (t timeRFC3339) MarshalJSON() (json []byte, err error) {
	tt := time.Time(t)
	return tt.MarshalJSON()
}

// MarshalText converts the timeRFC3339 to text in the form of a byte slice.
// It returns the byte slice and an error if any occurred during the conversion.
func (t timeRFC3339) MarshalText() (text []byte, err error) {
	tt := time.Time(t)
	return tt.MarshalText()
}

// UnmarshalJSON is a method that implements the json.Unmarshaler interface.
// It takes a JSON-encoded byte slice and converts it into a timeRFC3339 value.
func (t *timeRFC3339) UnmarshalJSON(data []byte) error {
	layout := utcLayoutJSON
	if tzOffsetRegex.Match(data) {
		layout = rfc3339JSON
	}
	return t.Parse(layout, string(data))
}

// UnmarshalText is a method of the timeRFC3339 type.
// The method attempts to parse the input data into a time value according to the RFC3339 standard.
// The method returns an error if the parsing fails.
func (t *timeRFC3339) UnmarshalText(data []byte) (err error) {
	layout := utcLayout
	if tzOffsetRegex.Match(data) {
		layout = time.RFC3339Nano
	}
	return t.Parse(layout, string(data))
}

// UnmarshalTimeString unmarshals a string representation of a time in RFC3339 format into a time.Time object.
func UnmarshalTimeString(ts string) *time.Time {
	logger := ucplog.FromContextOrDiscard(context.Background())
	var tt timeRFC3339
	err := tt.UnmarshalText([]byte(ts))
	if err != nil {
		logger.Info(fmt.Sprintf("Invalid time string: '%s'. Using default value", ts))
	}
	return (*time.Time)(&tt)
}

// Parse is a method of the timeRFC3339 type. It takes a layout and a value as strings,
// and attempts to parse the value into a time.Time object using the provided layout.
// The parsed time is then stored in the timeRFC3339 receiver. It returns any error produced by time.Parse.
func (t *timeRFC3339) Parse(layout, value string) error {
	p, err := time.Parse(layout, strings.ToUpper(value))
	*t = timeRFC3339(p)
	return err
}
