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

// Package awsapi has code the native AWS handlers share: AWS error codes, converting between a type's properties and
// AWS SDK structs, tags, and reading an operation's progress from a resource's state.
package awsapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
)

// Ambiguous is the error of a Find that matched more than one resource, which ids are, so UCP can't tell which one
// its lost create made. UCP then changes nothing: it keeps the record, and deletes and adopts none of them.
func Ambiguous(what string, ids []string) error {
	return &native.Failure{
		Message: fmt.Sprintf("AWS has %d %s that match the interrupted create (%s), so UCP can't tell which one it made and changed nothing. Remove the extra ones in AWS, then retry.", len(ids), what, strings.Join(ids, ", ")),
	}
}

// HasCode reports whether err is an AWS API error with one of codes, such as DBClusterNotFoundFault.
func HasCode(err error, codes ...string) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && slices.Contains(codes, apiErr.ErrorCode())
}

// NotFound returns err wrapped with native.ErrNotFound when it is an AWS API error with one of codes, and err
// otherwise.
func NotFound(err error, codes ...string) error {
	if err != nil && HasCode(err, codes...) {
		return fmt.Errorf("%w: %w", native.ErrNotFound, err)
	}
	return err
}

// Decode sets out, a pointer to an AWS SDK struct, from properties. Property names match the SDK's field names,
// ignoring case, as CloudFormation's names match the API's parameter names. Properties the struct has no field for
// are ignored.
func Decode(properties map[string]any, out any) error {
	b, err := json.Marshal(properties)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("failed to read the properties as %T: %w", out, err)
	}
	return nil
}

// Encode returns the properties of v, an AWS SDK struct or a slice of them, by field name. Unset fields are left out,
// and so are empty strings, lists, and objects, because AWS returns them for values the template never set. An empty
// tag value is kept, because a tag's value can be empty.
func Encode(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return prune(out), nil
}

// EncodeObject is Encode for a struct. It returns an empty map when v has no set fields.
func EncodeObject(v any) (map[string]any, error) {
	out, err := Encode(v)
	if err != nil {
		return nil, err
	}
	if m, ok := out.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

// prune removes nil values, empty strings, and empty lists and objects, and the SDK's ResultMetadata. Inside a Tags
// property it keeps empty strings, because a tag's value can be empty, and removes the property only when it is
// empty.
func prune(v any) any {
	return pruneValue(v, false)
}

func pruneValue(v any, inTags bool) any {
	switch t := v.(type) {
	case map[string]any:
		delete(t, "ResultMetadata")
		for k, value := range t {
			pruned := pruneValue(value, inTags || k == "Tags")
			if isEmpty(pruned, inTags) {
				delete(t, k)
			} else {
				t[k] = pruned
			}
		}
		return t
	case []any:
		out := make([]any, 0, len(t))
		for _, value := range t {
			if pruned := pruneValue(value, inTags); !isEmpty(pruned, inTags) {
				out = append(out, pruned)
			}
		}
		return out
	}
	return v
}

// isEmpty reports whether v is nil, or an empty list or object, or an empty string outside a Tags property.
func isEmpty(v any, inTags bool) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == "" && !inTags
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

// String returns the string property name of properties, or "" when it isn't a string.
func String(properties map[string]any, name string) string {
	s, _ := properties[name].(string)
	return s
}

// Strings returns the list of strings property name of properties. Values that aren't strings are left out.
func Strings(properties map[string]any, name string) []string {
	list, _ := properties[name].([]any)
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// Without returns a copy of properties without the named top-level properties.
func Without(properties map[string]any, names ...string) map[string]any {
	out := maps.Clone(properties)
	for _, name := range names {
		delete(out, name)
	}
	return out
}

// Changed returns the top-level properties the patch of request changes, such as EngineVersion for a patch that
// replaces /EngineVersion or /EngineVersion/Major.
func Changed(request native.UpdateRequest) []string {
	out := []string{}
	for _, op := range request.Patch {
		name, _, _ := strings.Cut(strings.TrimPrefix(op.Path, "/"), "/")
		if name != "" && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// OperationTag is the tag key a handler whose type finds a lost create by a tag (native.RecoverBySearch) puts on the
// resource, with the record's client token as the value. Read leaves it out, so it never counts as a change.
const OperationTag = "radius-operation-id"

// ReservedTags returns a rejected failure when the template's Tags set a key the handler keeps its own state in, such
// as OperationTag, and nil otherwise. A handler calls it before any AWS call of a create or an update, so a template
// can't change that state.
func ReservedTags(properties map[string]any, keys ...string) error {
	reserved := []string{}
	for _, tag := range Tags(properties) {
		if slices.Contains(keys, tag.Key) && !slices.Contains(reserved, tag.Key) {
			reserved = append(reserved, tag.Key)
		}
	}
	if len(reserved) == 0 {
		return nil
	}
	slices.Sort(reserved)
	return &native.Failure{
		Message:     fmt.Sprintf("Radius keeps its own state in these tags, so a template can't set them: %s. Remove them from Tags.", strings.Join(reserved, ", ")),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// Tag is a tag in CloudFormation's format.
type Tag struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

// Tags returns the Tags property of properties, in CloudFormation's list format.
func Tags(properties map[string]any) []Tag {
	list, _ := properties["Tags"].([]any)
	out := []Tag{}
	for _, item := range list {
		m, _ := item.(map[string]any)
		key, _ := m["Key"].(string)
		value, _ := m["Value"].(string)
		if key != "" {
			out = append(out, Tag{Key: key, Value: value})
		}
	}
	return out
}

// TagList returns tags for a resource's properties, without tags AWS or UCP add, sorted by key, or nil when none are
// left, so a read matches a template that sets no tags.
func TagList(tags map[string]string) []any {
	keys := slices.Sorted(maps.Keys(tags))
	out := []any{}
	for _, key := range keys {
		if strings.HasPrefix(key, "aws:") || key == OperationTag {
			continue
		}
		out = append(out, map[string]any{"Key": key, "Value": tags[key]})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// TagChanges returns the tags to set and the keys to remove to go from current to desired.
func TagChanges(current []Tag, desired []Tag) (set []Tag, remove []string) {
	want := map[string]string{}
	for _, tag := range desired {
		want[tag.Key] = tag.Value
	}
	have := map[string]string{}
	for _, tag := range current {
		have[tag.Key] = tag.Value
	}
	for _, key := range slices.Sorted(maps.Keys(want)) {
		if value, ok := have[key]; !ok || value != want[key] {
			set = append(set, Tag{Key: key, Value: want[key]})
		}
	}
	for _, key := range slices.Sorted(maps.Keys(have)) {
		if _, ok := want[key]; !ok {
			remove = append(remove, key)
		}
	}
	return set, remove
}

// States says what a resource's state means for an operation that waits for it.
type States struct {
	// Done are the states that finish the operation, such as "available". The first is the state UCP waits for.
	Done []string

	// Failed are the states that finish the operation with a failure, such as "failed".
	Failed []string
}

// Progress returns the progress of an operation whose resource reports state, compared ignoring case. Any other state
// means the operation is still running.
func (s States) Progress(state string) native.Progress {
	switch {
	case containsFold(s.Done, state):
		return native.Progress{Done: true, State: state}
	case containsFold(s.Failed, state):
		return native.Progress{Done: true, State: state, Failure: &native.Failure{Message: fmt.Sprintf("AWS reports the resource as %q", state)}}
	}
	waiting := ""
	if len(s.Done) > 0 {
		waiting = s.Done[0]
	}
	return native.Progress{State: state, Waiting: waiting}
}

func containsFold(list []string, state string) bool {
	return slices.ContainsFunc(list, func(v string) bool { return strings.EqualFold(v, state) })
}

// Calls makes calls in order, such as the AWS calls of an update, and stops at the first that fails. It returns the
// error of the first call as it is, so UCP treats the operation as rejected when AWS rejected the call as a client
// fault, and as unknown otherwise. It returns the error of a later call as a native.Failure whose outcome is partly
// applied, because the calls before it changed AWS.
func Calls(calls ...func() error) error {
	for i, call := range calls {
		err := call()
		if err == nil {
			continue
		}
		if i == 0 {
			return err
		}
		var failure *native.Failure
		if errors.As(err, &failure) {
			return err
		}
		return &native.Failure{Message: err.Error(), ClientError: isClientFault(err), Outcome: native.OutcomePartial}
	}
	return nil
}

func isClientFault(err error) bool {
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && (apiErr.ErrorFault() == smithy.FaultClient || apiErr.ErrorCode() == "ValidationException")
}

// FieldNames returns the names of the exported fields of v, a struct or a pointer to one, such as an AWS SDK input.
func FieldNames(v any) []string {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	out := []string{}
	for i := range t.NumField() {
		if f := t.Field(i); f.IsExported() {
			out = append(out, f.Name)
		}
	}
	return out
}
