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

// https://github.com/Azure/azure-sdk-for-go/blob/main/sdk/azcore/to

package to

// Ptr takes in a value of any type and returns a pointer to that value.
func Ptr[T any](v T) *T {
	return &v
}

// SliceOfPtrs takes in a variable number of arguments of any type and returns a slice of pointers to those arguments.
func SliceOfPtrs[T any](vv ...T) []*T {
	slc := make([]*T, len(vv))
	for i := range vv {
		slc[i] = Ptr(vv[i])
	}
	return slc
}

// StringMapPtr creates a new map with string keys and pointer values from an existing map with string keys and string values.
func StringMapPtr(ms map[string]string) *map[string]*string {
	msp := make(map[string]*string, len(ms))
	for k, s := range ms {
		msp[k] = Ptr(s)
	}
	return &msp
}
