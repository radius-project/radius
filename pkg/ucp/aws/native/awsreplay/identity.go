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

package awsreplay

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/smithy-go/encoding/cbor"
)

// replacement replaces old with new.
type replacement struct{ old, new string }

// identityReplacements returns the replacements of the recording identity's user ID and principal names, as is and
// percent-encoded, longest first, so a value that contains another is replaced whole. Each value is also matched as
// scrub leaves it, because scrub runs first. The root user's user ID is its account ID, which scrub replaces.
func (s *Session) identityReplacements() []replacement {
	if s.identity == nil {
		return nil
	}
	var rs []replacement
	add := func(v, new string) {
		if v == "" || v == aws.ToString(s.identity.Account) {
			return
		}
		for _, v := range []string{v, string(s.scrub([]byte(v)))} {
			for _, form := range escapes(v) {
				rs = append(rs, replacement{form, new})
			}
		}
	}
	add(aws.ToString(s.identity.UserId), UserID)
	for _, name := range principalNames(aws.ToString(s.identity.Arn)) {
		add(name, Principal)
	}
	slices.SortStableFunc(rs, func(a, b replacement) int { return len(b.old) - len(a.old) })
	return rs
}

// principalNames returns the names in the ARN of a caller identity: the user name of an IAM user, the role and session
// names of an assumed role, and the name of a federated user. The root user has none.
func principalNames(identityARN string) []string {
	parsed, err := arn.Parse(identityARN)
	if err != nil {
		return nil
	}
	kind, names, ok := strings.Cut(parsed.Resource, "/")
	if !ok {
		return nil
	}
	if kind == "assumed-role" {
		return strings.Split(names, "/")
	}
	return []string{names[strings.LastIndex(names, "/")+1:]}
}

// scrubIdentity returns b with the recording identity replaced in its string values: JSON strings, XML text, form
// values, and CBOR text strings. It never changes a key or the syntax, so the body still decodes. contentType tells a
// CBOR or form body apart. It returns an error when b holds the identity somewhere it can't replace it, such as an XML
// attribute or a binary body that isn't CBOR.
func (s *Session) scrubIdentity(b []byte, contentType string) ([]byte, error) {
	rs := s.identityReplacements()
	if len(rs) == 0 || len(b) == 0 {
		return b, nil
	}
	replace := func(v string) string { return replaceNames(v, rs) }
	out, err := scrubValues(b, contentType, replace)
	var held *heldError
	if err != nil && !errors.As(err, &held) && replace(string(b)) == string(b) {
		// A body that doesn't parse, and doesn't hold the identity either, is kept as is.
		return b, nil
	}
	return out, err
}

// heldError is the error for a body that holds the recording identity somewhere scrubIdentity can't replace it.
type heldError struct{ where string }

func (e *heldError) Error() string {
	return e.where + " holds the recording identity"
}

// scrubValues returns b with replace applied to each of its string values.
func scrubValues(b []byte, contentType string, replace func(string) string) ([]byte, error) {
	switch {
	case strings.Contains(contentType, "cbor"):
		return scrubCBOR(b, replace)
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		values, err := url.ParseQuery(string(b))
		if err != nil {
			return nil, err
		}
		for _, vs := range values {
			for i := range vs {
				vs[i] = replace(vs[i])
			}
		}
		return []byte(values.Encode()), nil
	case json.Valid(b):
		return scrubJSON(b, replace), nil
	case bytes.HasPrefix(bytes.TrimSpace(b), []byte("<")):
		return scrubXML(b, replace)
	case utf8.Valid(b), replace(string(b)) == string(b):
		return []byte(replace(string(b))), nil
	}
	return nil, &heldError{"a binary body that isn't CBOR"}
}

// replaceNames returns v with each replacement made where its old value is a whole name: not next to a character an
// IAM name can contain, as is or percent-encoded. So a short name such as "a" is replaced in "a" and "user/a", but not
// in "available".
func replaceNames(v string, rs []replacement) string {
	var out strings.Builder
	changed := false
next:
	for i := 0; i < len(v); {
		if nameStart(v, i) {
			for _, r := range rs {
				if end := i + len(r.old); strings.HasPrefix(v[i:], r.old) && nameEnd(v, end) {
					out.WriteString(r.new)
					i, changed = end, true
					continue next
				}
			}
		}
		out.WriteByte(v[i])
		i++
	}
	if !changed {
		return v
	}
	return out.String()
}

// nameStart reports whether a name can start at v[i], which isn't inside a percent escape.
func nameStart(v string, i int) bool {
	if _, ok := percentEscape(v, i-1); ok {
		return false
	}
	if _, ok := percentEscape(v, i-2); ok {
		return false
	}
	if i == 0 || !isNameChar(v[i-1]) {
		return true
	}
	c, ok := percentEscape(v, i-3)
	return ok && !isNameChar(c)
}

// nameEnd reports whether a name can end before v[i].
func nameEnd(v string, i int) bool {
	if i == len(v) {
		return true
	}
	if c, ok := percentEscape(v, i); ok {
		return !isNameChar(c)
	}
	return !isNameChar(v[i])
}

// isNameChar reports whether an IAM user, role, or session name can contain c.
func isNameChar(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("_+=,.@-", c) >= 0
}

// percentEscape returns the byte a percent escape at v[i:] stands for.
func percentEscape(v string, i int) (byte, bool) {
	if i < 0 || i+3 > len(v) || v[i] != '%' {
		return 0, false
	}
	b, err := hex.DecodeString(v[i+1 : i+3])
	if err != nil {
		return 0, false
	}
	return b[0], true
}

// scrubJSON returns the valid JSON b with replace applied to each string that isn't an object key. The rest of b is
// kept byte for byte.
func scrubJSON(b []byte, replace func(string) string) []byte {
	var out []byte
	last := 0
	for i := 0; i < len(b); i++ {
		if b[i] != '"' {
			continue
		}
		end := i + 1
		for b[end] != '"' {
			if b[end] == '\\' {
				end++
			}
			end++
		}
		end++
		next := end
		for next < len(b) && strings.IndexByte(" \t\r\n", b[next]) >= 0 {
			next++
		}
		if next == len(b) || b[next] != ':' {
			var v string
			_ = json.Unmarshal(b[i:end], &v)
			if scrubbed := replace(v); scrubbed != v {
				out = append(append(out, b[last:i]...), jsonString(scrubbed)...)
				last = end
			}
		}
		i = end - 1
	}
	return append(out, b[last:]...)
}

// jsonString returns v as a JSON string, without escaping HTML characters, which AWS doesn't escape either.
func jsonString(v string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// scrubXML returns b with replace applied to the text of each element, with adjacent text and CDATA sections read as
// one. The rest of b is kept byte for byte.
func scrubXML(b []byte, replace func(string) string) ([]byte, error) {
	d := xml.NewDecoder(bytes.NewReader(b))
	var out bytes.Buffer
	var last int64
	runStart, runEnd := int64(-1), int64(0)
	var run strings.Builder
	flush := func() error {
		if runStart < 0 {
			return nil
		}
		text := run.String()
		start := runStart
		run.Reset()
		runStart = -1
		if v := replace(text); v != text {
			out.Write(b[last:start])
			if err := xml.EscapeText(&out, []byte(v)); err != nil {
				return err
			}
			last = runEnd
		}
		return nil
	}
	for {
		start := d.InputOffset()
		tok, err := d.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if text, ok := tok.(xml.CharData); ok {
			if runStart < 0 {
				runStart = start
			}
			run.Write(text)
			runEnd = d.InputOffset()
			continue
		}
		if comment, ok := tok.(xml.Comment); ok {
			if replace(string(comment)) != string(comment) {
				return nil, &heldError{"an XML comment"}
			}
			// A comment doesn't end the text of an element.
			continue
		}
		if err := flush(); err != nil {
			return nil, err
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			for _, a := range tok.Attr {
				// A namespace is part of the protocol, such as https://sts.amazonaws.com/doc/2011-06-15/.
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
					continue
				}
				if replace(a.Value) != a.Value {
					return nil, &heldError{"the XML attribute " + a.Name.Local}
				}
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	out.Write(b[last:])
	return out.Bytes(), nil
}

// scrubCBOR returns b with replace applied to each text string that isn't a map key. b is kept as is when nothing
// changes, and encoded again when something does.
func scrubCBOR(b []byte, replace func(string) string) ([]byte, error) {
	v, err := cbor.Decode(b)
	if err != nil {
		return nil, err
	}
	if !scrubCBORValue(&v, replace) {
		return b, nil
	}
	return cbor.Encode(v), nil
}

// scrubCBORValue applies replace to the text strings in *v, and reports whether any changed.
func scrubCBORValue(v *cbor.Value, replace func(string) string) bool {
	changed := false
	switch value := (*v).(type) {
	case cbor.String:
		if scrubbed := replace(string(value)); scrubbed != string(value) {
			*v, changed = cbor.String(scrubbed), true
		}
	case cbor.List:
		for i := range value {
			changed = scrubCBORValue(&value[i], replace) || changed
		}
	case cbor.Map:
		for k, e := range value {
			if scrubCBORValue(&e, replace) {
				value[k], changed = e, true
			}
		}
	case *cbor.Tag:
		changed = scrubCBORValue(&value.Value, replace)
	}
	return changed
}
