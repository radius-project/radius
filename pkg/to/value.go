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

// Package to provides helpers to ease working with pointer values of marshalled structures.

package to

//

// String returns the string pointed to by s if s is not nil, otherwise it returns an empty string.
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

// StringMap takes in a map of strings and pointers to strings and returns a map of strings with empty strings in place of
// nil pointers.
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

// Bool returns the boolean value of the pointer passed in, or false if the pointer is nil.
func Bool(b *bool) bool {
	if b != nil {
		return *b
	}
	return false
}

// Int returns the value of the pointer if it is not nil, otherwise it returns 0.
func Int(i *int) int {
	if i != nil {
		return *i
	}
	return 0
}

// Int32 checks if the pointer to an int32 is nil and returns the int32 value if it is not nil, otherwise it returns 0.
func Int32(i *int32) int32 {
	if i != nil {
		return *i
	}
	return 0
}

// Int64 returns the int64 value of the pointer passed in, or 0 if the pointer is nil.
func Int64(i *int64) int64 {
	if i != nil {
		return *i
	}
	return 0
}

// Float32 returns the value of the float32 pointer if it is not nil, otherwise it returns 0.0.
func Float32(i *float32) float32 {
	if i != nil {
		return *i
	}
	return 0.0
}

// Float64 returns the value of the float64 pointer if it is not nil, otherwise it returns 0.0.
func Float64(i *float64) float64 {
	if i != nil {
		return *i
	}
	return 0.0
}
