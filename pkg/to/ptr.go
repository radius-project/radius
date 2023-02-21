// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
