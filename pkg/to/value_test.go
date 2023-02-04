// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package to

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	v := ""
	require.Equal(t, v, String(&v))
	require.Equal(t, "", String(nil))
	require.Equal(t, v, *Ptr(v))
}

func TestStringSlice(t *testing.T) {
	v := []string{}
	require.Equal(t, StringSlice(&v), v)
	require.Nil(t, StringSlice(nil))
	w := []string{"a", "b"}
	require.Equal(t, Ptr(w), w)
}

func TestStringMap(t *testing.T) {
	msp := map[string]*string{"foo": Ptr("foo"), "bar": Ptr("bar"), "baz": Ptr("baz")}
	for k, v := range StringMap(msp) {
		require.Equal(t, *msp[k], v)
	}
}

func TestStringMapHandlesNil(t *testing.T) {
	msp := map[string]*string{"foo": Ptr("foo"), "bar": nil, "baz": Ptr("baz")}
	for k, v := range StringMap(msp) {
		if msp[k] != nil {
			require.Equal(t, *msp[k], v)
		}
	}
}

func TestStringMapPtr(t *testing.T) {
	ms := map[string]string{"foo": "foo", "bar": "bar", "baz": "baz"}
	for k, msp := range *StringMapPtr(ms) {
		require.Equal(t, ms[k], *msp)
	}
}

func TestBool(t *testing.T) {
	v := false
	require.Equal(t, v, Bool(&v))
	require.Equal(t, false, Bool(nil))
	w := false
	require.Equal(t, w, *Ptr(w))
}

func TestInt(t *testing.T) {
	v := 0
	require.Equal(t, v, Int(&v))
	require.Equal(t, 0, Int(nil))
	w := 0
	require.Equal(t, w, *Ptr(w))
}

func TestInt32(t *testing.T) {
	v := int32(0)
	require.Equal(t, v, Int32(&v))
	require.Equal(t, 0, Int32(nil))
	w := 0
	require.Equal(t, w, *Ptr(w))
}

func TestInt64(t *testing.T) {
	v := int64(0)
	require.Equal(t, v, Int64(&v))
	require.Equal(t, 0, Int64(nil))
	w := 0
	require.Equal(t, w, *Ptr(w))
}

func TestFloat32(t *testing.T) {
	v := float32(0)
	require.Equal(t, v, Float32(&v))
	require.Equal(t, 0, Float32(nil))
	w := 0
	require.Equal(t, w, *Ptr(w))
}

func TestFloat64(t *testing.T) {
	v := float64(0)
	require.Equal(t, v, Float64(&v))
	require.Equal(t, 0, Float64(nil))
	w := 0
	require.Equal(t, w, *Ptr(w))
}

func TestByteSlicePtr(t *testing.T) {
	v := []byte("bytes")
	require.Equal(t, v, *Ptr(v))
}
