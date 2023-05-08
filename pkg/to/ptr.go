/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

// https://github.com/Azure/azure-sdk-for-go/blob/main/sdk/azcore/to

package to

// Ptr returns a pointer to the provided value.
func Ptr[T any](v T) *T {
	return &v
}

// SliceOfPtrs returns a slice of *T from the specified values.
func SliceOfPtrs[T any](vv ...T) []*T {
	slc := make([]*T, len(vv))
	for i := range vv {
		slc[i] = Ptr(vv[i])
	}
	return slc
}

// StringMapPtr returns a pointer to a map of string pointers built from the passed map of strings.
func StringMapPtr(ms map[string]string) *map[string]*string {
	msp := make(map[string]*string, len(ms))
	for k, s := range ms {
		msp[k] = Ptr(s)
	}
	return &msp
}
