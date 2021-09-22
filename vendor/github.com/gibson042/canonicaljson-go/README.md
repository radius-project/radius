# canonicaljson-go
Go library for producing JSON in canonical format as specified by [https://gibson042.github.io/canonicaljson-spec/](https://gibson042.github.io/canonicaljson-spec/).
The provided interface matches that of standard package "[encoding/json](https://golang.org/pkg/encoding/json/)" wherever they overlap:
* [func `Marshal`](https://golang.org/pkg/encoding/json/#Marshal)
* [func `MarshalIndent`](https://golang.org/pkg/encoding/json/#MarshalIndent)
* [type `Encoder`](https://golang.org/pkg/encoding/json/#Encoder)

Types from the standard package are also accepted wherever relevant.

Test this package by invoking `test.sh`.

```
godoc github.com/gibson042/canonicaljson-go

PACKAGE DOCUMENTATION

package canonicaljson
    import "github.com/gibson042/canonicaljson-go"

    Package canonicaljson implements canonical serialization of Go objects
    to canonical-form JSON as specified at
    https://gibson042.github.io/canonicaljson-spec/ . The provided interface
    should match that of standard package "encoding/json" (from which it is
    derived) wherever they overlap (and in fact, this package is essentially
    a 2016-03-09 fork from
    golang/go@9d77ad8d34ce56e182adc30cd21af50a4b00932c:src/encoding/json ).
    Notable differences:

	- Object keys are sorted lexicographically by code point
	- Non-integer JSON numbers are represented in capital-E exponential
	  notation with significand in (-10, 10) and no insignificant signs
	  or zeroes beyond those required to force a decimal point.
	- JSON strings are represented in UTF-8 with minimal byte length,
	  using escapes only when necessary for validity and Unicode
	  escapes (uppercase hex) only when there is no shorter option.

FUNCTIONS

func Marshal(v interface{}) ([]byte, error)
    Marshal returns the canonical UTF-8 JSON encoding of v.

    Marshal traverses the value v recursively. If an encountered value
    implements the json.Marshaler interface and is not a nil pointer,
    Marshal calls its MarshalJSON method to produce JSON. If no MarshalJSON
    method is present but the value implements encoding.TextMarshaler
    instead, Marshal calls its MarshalText method. The nil pointer exception
    is not strictly necessary but mimics a similar, necessary exception in
    the behavior of json.UnmarshalJSON.

    Otherwise, Marshal uses the following type-dependent default encodings:

    Boolean values encode as JSON booleans.

    Floating point, integer, and Number values encode as JSON numbers.
    Non-fractional values become sequences of digits without leading spaces;
    fractional values are represented in capital-E exponential notation with
    the shortest possible significand of magnitude less than 10 that
    includes at least one digit both before and after the decimal point, and
    the shortest possible non-empty exponent.

    String values encode as JSON strings, treating ill-formed UTF-8 input as
    an error (with the exception of "WTF-8" encodings of lone
    surrogates—code points U+D800 through U+DFFF [inclusive] that could not
    be interpreted as part of a surrogate pair). This is in contrast to
    encoding/json, which replaces ill-formed sequences with U+FFFD
    REPLACEMENT CHARACTER. Also in contrast to encoding/json, characters
    appear unescaped whenever possible. Control characters U+0000 through
    U+001F and lone surrogates U+D800 through U+DFFF are replaced with their
    shortest escape sequence, 4 uppercase hex characters except for the
    following:

	- \b U+0008 BACKSPACE
	- \t U+0009 CHARACTER TABULATION ("tab")
	- \n U+000A LINE FEED ("newline")
	- \f U+000C FORM FEED
	- \r U+000D CARRIAGE RETURN

    Array and slice values encode as JSON arrays, except that []byte encodes
    as a base64-encoded string, and a nil slice encodes as the null JSON
    object.

    Struct values encode as JSON objects. Each exported struct field becomes
    a member of the object unless

	- the field's tag is "-", or
	- the field is empty and its tag specifies the "omitempty" option.

    The empty values are false, 0, any nil pointer or interface value, and
    any array, slice, map, or string of length zero. The object's default
    key string is the struct field name but can be specified in the struct
    field's tag value. The "json" key in the struct field's tag value is the
    key name, followed by an optional comma and options. Examples:

	// Field is ignored by this package.
	Field int `json:"-"`

	// Field appears in JSON as key "myName".
	Field int `json:"myName"`

	// Field appears in JSON as key "myName" and
	// the field is omitted from the object if its value is empty,
	// as defined above.
	Field int `json:"myName,omitempty"`

	// Field appears in JSON as key "Field" (the default), but
	// the field is skipped if empty.
	// Note the leading comma.
	Field int `json:",omitempty"`

    The "string" option signals that a field is stored as JSON inside a
    JSON-encoded string. It applies only to fields of string, floating
    point, integer, or boolean types. This extra level of encoding is
    sometimes used when communicating with JavaScript programs:

	Int64String int64 `json:",string"`

    The key name will be used if it's a non-empty string consisting of only
    Unicode letters, digits, dollar signs, percent signs, hyphens,
    underscores and slashes.

    Anonymous struct fields are usually marshaled as if their inner exported
    fields were fields in the outer struct, subject to the usual Go
    visibility rules amended as described in the next paragraph. An
    anonymous struct field with a name given in its JSON tag is treated as
    having that name, rather than being anonymous. An anonymous struct field
    of interface type is treated the same as having that type as its name,
    rather than being anonymous.

    The Go visibility rules for struct fields are amended for JSON when
    deciding which field to marshal. If there are multiple fields at the
    same level, and that level is the least nested (and would therefore be
    the nesting level selected by the usual Go rules), the following extra
    rules apply:

    1) Of those fields, if any are JSON-tagged, only tagged fields are
    considered, even if there are multiple untagged fields that would
    otherwise conflict. 2) If there is exactly one field (tagged or not
    according to the first rule), that is selected. 3) Otherwise there are
    multiple fields, and all are ignored; no error occurs.

    Handling of anonymous struct fields is new in Go 1.1. Prior to Go 1.1,
    anonymous struct fields were ignored. To force ignoring of an anonymous
    struct field in both current and earlier versions, give the field a JSON
    tag of "-".

    Map values encode as JSON objects. The map's key type must be string;
    the map keys are used as JSON object keys, subject to the UTF-8 coercion
    described for string values above.

    Pointer values encode as the value pointed to. A nil pointer encodes as
    the null JSON object.

    Interface values encode as the value contained in the interface. A nil
    interface value encodes as the null JSON object.

    Channel, complex, and function values cannot be encoded in JSON.
    Attempting to encode such a value causes Marshal to return an
    UnsupportedTypeError.

    JSON cannot represent cyclic data structures and Marshal does not handle
    them. Passing cyclic structures to Marshal will result in an infinite
    recursion.

func MarshalIndent(v interface{}, prefix, indent string) ([]byte, error)
    MarshalIndent is like Marshal, but adds whitespace for more readable
    output.

func Unmarshal(data []byte, v interface{}) error
    Unmarshal parses the JSON-encoded UTF-8 data and stores the result in
    the value pointed to by v.

    Unmarshal uses the inverse of the encodings that Marshal uses,
    allocating maps, slices, and pointers as necessary, with the following
    additional rules:

    To unmarshal JSON into a pointer, Unmarshal first handles the case of
    the JSON being the JSON literal null. In that case, Unmarshal sets the
    pointer to nil. Otherwise, Unmarshal unmarshals the JSON into the value
    pointed at by the pointer. If the pointer is nil, Unmarshal allocates a
    new value for it to point to.

    To unmarshal JSON into a struct, Unmarshal matches incoming object keys
    to the keys used by Marshal (either the struct field name or its tag),
    preferring an exact match but also accepting a case-insensitive match.
    Unmarshal will only set exported fields of the struct.

    To unmarshal JSON into an interface value, Unmarshal stores one of these
    in the interface value:

	bool, for JSON booleans
	float64, for JSON numbers
	string, for JSON strings
	[]interface{}, for JSON arrays
	map[string]interface{}, for JSON objects
	nil for JSON null

    To unmarshal a JSON array into a slice, Unmarshal resets the slice
    length to zero and then appends each element to the slice. As a special
    case, to unmarshal an empty JSON array into a slice, Unmarshal replaces
    the slice with a new empty slice.

    To unmarshal a JSON array into a Go array, Unmarshal decodes JSON array
    elements into corresponding Go array elements. If the Go array is
    smaller than the JSON array, the additional JSON array elements are
    discarded. If the JSON array is smaller than the Go array, the
    additional Go array elements are set to zero values.

    To unmarshal a JSON object into a string-keyed map, Unmarshal first
    establishes a map to use, If the map is nil, Unmarshal allocates a new
    map. Otherwise Unmarshal reuses the existing map, keeping existing
    entries. Unmarshal then stores key-value pairs from the JSON object into
    the map.

    If a JSON value is not appropriate for a given target type, or if a JSON
    number overflows the target type, Unmarshal skips that field and
    completes the unmarshaling as best it can. If no more serious errors are
    encountered, Unmarshal returns an UnmarshalTypeError describing the
    earliest such error.

    The JSON null value unmarshals into an interface, map, pointer, or slice
    by setting that Go value to nil. Because null is often used in JSON to
    mean ``not present,'' unmarshaling a JSON null into any other Go type
    has no effect on the value and produces no error.

    Invalid UTF-8 input is always treated as an error, even if it is
    encountered when unmarshaling a quoted string (in contrast to
    encoding/json, which replaces bad octets with U+FFFD REPLACEMENT
    CHARACTER). However, `\uXXXX` JSON string escape sequences specifying
    lone surrogates (code points U+D800 through U+DFFF [inclusive] that are
    not part of a surrogate pair) are interpreted to produce "WTF-8" runes
    that cannot be losslessly serialized to UTF-8 and might produce
    unexpected behavior if passed to functions expecting all strings to
    contain valid UTF-8. This package's Marshal function checks for such
    runes and emits them as valid JSON escape sequences.

TYPES

type Decoder struct {
    // contains filtered or unexported fields
}
    A Decoder reads and decodes JSON objects from an input stream.

func NewDecoder(r io.Reader) *Decoder
    NewDecoder returns a new decoder that reads from r.

    The decoder introduces its own buffering and may read data from r beyond
    the JSON values requested.

func (dec *Decoder) Buffered() io.Reader
    Buffered returns a reader of the data remaining in the Decoder's buffer.
    The reader is valid until the next call to Decode.

func (dec *Decoder) Decode(v interface{}) error
    Decode reads the next JSON-encoded value from its input and stores it in
    the value pointed to by v.

    See the documentation for Unmarshal for details about the conversion of
    JSON into a Go value.

func (dec *Decoder) More() bool
    More reports whether there is another element in the current array or
    object being parsed.

func (dec *Decoder) Token() (Token, error)
    Token returns the next JSON token in the input stream. At the end of the
    input stream, Token returns nil, io.EOF.

    Token guarantees that the delimiters [ ] { } it returns are properly
    nested and matched: if Token encounters an unexpected delimiter in the
    input, it will return an error.

    The input stream consists of basic JSON values—bool, string, number, and
    null—along with delimiters [ ] { } of type Delim to mark the start and
    end of arrays and objects. Commas and colons are elided.

func (dec *Decoder) UseNumber()
    UseNumber causes the Decoder to unmarshal a number into an interface{}
    as a Number instead of as a float64.

type Delim rune
    A Delim is a JSON array or object delimiter, one of [ ] { or }.

func (d Delim) String() string

type Encoder struct {
    // contains filtered or unexported fields
}
    An Encoder writes JSON objects to an output stream.

func NewEncoder(w io.Writer) *Encoder
    NewEncoder returns a new encoder that writes to w.

func (enc *Encoder) Encode(v interface{}) error
    Encode writes the JSON encoding of v to the stream, followed by a
    newline character.

    See the documentation for Marshal for details about the conversion of Go
    values to JSON.

type InvalidUnmarshalError struct {
    Type reflect.Type
}
    An InvalidUnmarshalError describes an invalid argument passed to
    Unmarshal. (The argument to Unmarshal must be a non-nil pointer.)

func (e *InvalidUnmarshalError) Error() string

type Marshaler interface {
    MarshalJSON() ([]byte, error)
}
    Marshaler is the interface implemented by objects that can marshal
    themselves into valid JSON.

type MarshalerError struct {
    Type reflect.Type
    Err  error
}

func (e *MarshalerError) Error() string

type Number string
    A Number represents a JSON number literal. TODO(go>=1.9): type Number =
    json.Number

func (n Number) Float64() (float64, error)
    Float64 returns the number as a float64.

func (n Number) Int64() (int64, error)
    Int64 returns the number as an int64.

func (n Number) String() string
    String returns the literal text of the number.

type RawMessage []byte
    RawMessage is a raw encoded JSON object. It implements Marshaler and
    Unmarshaler and can be used to delay JSON decoding or precompute a JSON
    encoding.

func (m *RawMessage) MarshalJSON() ([]byte, error)
    MarshalJSON returns *m as the JSON encoding of m.

func (m *RawMessage) UnmarshalJSON(data []byte) error
    UnmarshalJSON sets *m to a copy of data.

type SyntaxError struct {
    Offset int64 // error occurred after reading Offset bytes
    // contains filtered or unexported fields
}
    A SyntaxError is a description of a JSON syntax error.

func (e *SyntaxError) Error() string

type Token interface{}
    A Token holds a value of one of these types:

	Delim, for the four JSON delimiters [ ] { }
	bool, for JSON booleans
	float64, for JSON numbers
	Number, for JSON numbers
	string, for JSON string literals
	nil, for JSON null

type UnmarshalFieldError struct {
    Key   string
    Type  reflect.Type
    Field reflect.StructField
}
    An UnmarshalFieldError describes a JSON object key that led to an
    unexported (and therefore unwritable) struct field. (No longer used;
    kept for compatibility.)

func (e *UnmarshalFieldError) Error() string

type UnmarshalTypeError struct {
    Value  string       // description of JSON value - "bool", "array", "number -5"
    Type   reflect.Type // type of Go value it could not be assigned to
    Offset int64        // error occurred after reading Offset bytes
}
    An UnmarshalTypeError describes a JSON value that was not appropriate
    for a value of a specific Go type.

func (e *UnmarshalTypeError) Error() string

type Unmarshaler interface {
    UnmarshalJSON([]byte) error
}
    Unmarshaler is the interface implemented by objects that can unmarshal a
    JSON description of themselves. The input can be assumed to be a valid
    encoding of a JSON value. UnmarshalJSON must copy the JSON data if it
    wishes to retain the data after returning.

type UnsupportedTypeError struct {
    Type reflect.Type
}
    An UnsupportedTypeError is returned by Marshal when attempting to encode
    an unsupported value type.

func (e *UnsupportedTypeError) Error() string

type UnsupportedValueError struct {
    Value reflect.Value
    Str   string
}

func (e *UnsupportedValueError) Error() string

SUBDIRECTORIES

	canonicaljson-spec
	cli
```
