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

package to

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestString(t *testing.T) {
	v := ""
	require.Exactly(t, v, String(&v))
	require.Exactly(t, "", String(nil))
	require.Exactly(t, v, *Ptr(v))
}

func TestStringSlice(t *testing.T) {
	v := []string{}
	require.Exactly(t, v, StringSlice(&v))
	require.Nil(t, StringSlice(nil))
	w := []string{"a", "b"}
	require.Exactly(t, w, *Ptr(w))
}

func TestStringMap(t *testing.T) {
	msp := map[string]*string{"foo": Ptr("foo"), "bar": Ptr("bar"), "baz": Ptr("baz")}
	for k, v := range StringMap(msp) {
		require.Exactly(t, *msp[k], v)
	}
}

func TestStringMapHandlesNil(t *testing.T) {
	msp := map[string]*string{"foo": Ptr("foo"), "bar": nil, "baz": Ptr("baz")}
	for k, v := range StringMap(msp) {
		if msp[k] != nil {
			require.Exactly(t, *msp[k], v)
		}
	}
}

func TestStringMapPtr(t *testing.T) {
	ms := map[string]string{"foo": "foo", "bar": "bar", "baz": "baz"}
	for k, msp := range *StringMapPtr(ms) {
		require.Exactly(t, ms[k], *msp)
	}
}

func TestBool(t *testing.T) {
	v := false
	require.Exactly(t, v, Bool(&v))
	require.Exactly(t, false, Bool(nil))
	require.Exactly(t, v, *Ptr(v))
}

func TestInt(t *testing.T) {
	v := 0
	require.Equal(t, v, Int(&v))
	require.Equal(t, 0, Int(nil))
	require.Equal(t, v, *Ptr(v))
}

func TestInt32(t *testing.T) {
	v := int32(0)
	require.Exactly(t, v, Int32(&v))
	require.Exactly(t, int32(0), Int32(nil))
	require.Exactly(t, v, *Ptr(v))
}

func TestInt64(t *testing.T) {
	v := int64(0)
	require.Exactly(t, v, Int64(&v))
	require.Exactly(t, int64(0), Int64(nil))
	require.Exactly(t, v, *Ptr(v))
}

func TestFloat32(t *testing.T) {
	v := float32(0)
	require.Equal(t, v, Float32(&v))
	require.Exactly(t, float32(0), Float32(nil))
	require.Equal(t, v, *Ptr(v))
}

func TestFloat64(t *testing.T) {
	v := float64(0)
	require.Equal(t, v, Float64(&v))
	require.Exactly(t, float64(0), Float64(nil))
	require.Equal(t, v, *Ptr(v))
}

func TestByteSlicePtr(t *testing.T) {
	v := []byte("bytes")
	require.Equal(t, v, *Ptr(v))
}
