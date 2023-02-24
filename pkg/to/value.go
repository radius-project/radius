// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Package to provides helpers to ease working with pointer values of marshalled structures.

package to

// String returns a string value for the passed string pointer. It returns the empty string if the
// pointer is nil.
func String(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

// StringSlice returns a string slice value for the passed string slice pointer. It returns a nil
// slice if the pointer is nil.
func StringSlice(s *[]string) []string {
	if s != nil {
		return *s
	}
	return nil
}

// StringMap returns a map of strings built from the map of string pointers. The empty string is
// used for nil pointers.
func StringMap(msp map[string]*string) map[string]string {
	ms := make(map[string]string, len(msp))
	for k, sp := range msp {
		if sp != nil {
			ms[k] = *sp
		} else {
			ms[k] = ""
		}
	}
	return ms
}

// Bool returns a bool value for the passed bool pointer. It returns false if the pointer is nil.
func Bool(b *bool) bool {
	if b != nil {
		return *b
	}
	return false
}

// Int returns an int value for the passed int pointer. It returns 0 if the pointer is nil.
func Int(i *int) int {
	if i != nil {
		return *i
	}
	return 0
}

// Int32 returns an int value for the passed int pointer. It returns 0 if the pointer is nil.
func Int32(i *int32) int32 {
	if i != nil {
		return *i
	}
	return 0
}

// Int64 returns an int value for the passed int pointer. It returns 0 if the pointer is nil.
func Int64(i *int64) int64 {
	if i != nil {
		return *i
	}
	return 0
}

// Float32 returns an int value for the passed int pointer. It returns 0.0 if the pointer is nil.
func Float32(i *float32) float32 {
	if i != nil {
		return *i
	}
	return 0.0
}

// Float64 returns an int value for the passed int pointer. It returns 0.0 if the pointer is nil.
func Float64(i *float64) float64 {
	if i != nil {
		return *i
	}
	return 0.0
}
