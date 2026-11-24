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

package redshiftserverless

import (
	"context"
	"math"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	usageLimitType = "AWS::RedshiftServerless::UsageLimit"

	// defaultPeriod and defaultBreachAction are what AWS uses when a create leaves Period and BreachAction out.
	defaultPeriod       = string(types.UsageLimitPeriodMonthly)
	defaultBreachAction = string(types.UsageLimitBreachActionLog)

	// maxAmount is the largest Amount the handler sends: the largest integer a JSON number holds exactly.
	maxAmount = 1 << 53
)

var (
	_ native.Handler           = (*UsageLimit)(nil)
	_ native.IdentifierEncoder = (*UsageLimit)(nil)
	_ native.Finder            = (*UsageLimit)(nil)
)

// UsageLimit serves AWS::RedshiftServerless::UsageLimit. Its identifier is UsageLimitId, which AWS assigns.
// CreateUsageLimit takes no idempotency token, usage limits can't be tagged, and AWS accepts two limits with the same
// values, so UCP finds a create it lost by its values, and fails when more than one limit has them.
type UsageLimit struct {
	api API
}

// NewUsageLimit returns the handler for Redshift Serverless usage limits.
func NewUsageLimit(api API) *UsageLimit {
	return &UsageLimit{api: api}
}

// TypeName returns AWS::RedshiftServerless::UsageLimit.
func (h *UsageLimit) TypeName() string {
	return usageLimitType
}

// Options returns how UCP uses the handler.
func (h *UsageLimit) Options() native.Options {
	return native.Options{List: native.ListRegion, Recovery: native.RecoverBySearch}
}

// Identifier checks the template's values, so UCP rejects one AWS wouldn't take before any AWS call, and returns
// UsageLimitId, which only AWS's properties give, because AWS assigns it.
func (h *UsageLimit) Identifier(properties map[string]any) (string, error) {
	if err := validateUsageLimit(properties); err != nil {
		return "", err
	}
	return awsapi.String(properties, "UsageLimitId"), nil
}

// validateUsageLimit checks that each value of the template has its schema's type and is one AWS takes.
func validateUsageLimit(properties map[string]any) error {
	if err := checkStrings(usageLimitType, properties, "ResourceArn", "UsageType", "Period", "BreachAction"); err != nil {
		return err
	}
	for name, values := range map[string][]string{
		"UsageType":    enumValues(types.UsageLimitUsageType("").Values()),
		"Period":       enumValues(types.UsageLimitPeriod("").Values()),
		"BreachAction": enumValues(types.UsageLimitBreachAction("").Values()),
	} {
		if v, ok := properties[name].(string); ok && !slices.Contains(values, v) {
			return invalid(usageLimitType, name, "one of "+joinValues(values))
		}
	}
	if value, ok := properties["Amount"]; ok && value != nil {
		if _, ok := amount(value); !ok {
			return invalid(usageLimitType, "Amount", "a whole number from 1")
		}
	}
	return nil
}

// amount returns the Amount v gives, or false when it isn't a whole number from 1 to maxAmount.
func amount(v any) (int64, bool) {
	var n int64
	switch value := v.(type) {
	case float64:
		if value != math.Trunc(value) || value < 1 || value > maxAmount {
			return 0, false
		}
		n = int64(value)
	case int:
		n = int64(value)
	case int64:
		n = value
	default:
		return 0, false
	}
	return n, n >= 1 && n <= maxAmount
}

// enumValues returns the values of an SDK enum as strings.
func enumValues[T ~string](values []T) []string {
	out := []string{}
	for _, v := range values {
		out = append(out, string(v))
	}
	return out
}

// joinValues returns values as a list for a message, such as "log, emit-metric, or deactivate".
func joinValues(values []string) string {
	out := ""
	for i, v := range values {
		switch {
		case i == 0:
		case i == len(values)-1:
			out += ", or "
		default:
			out += ", "
		}
		out += v
	}
	return out
}

// usageLimitProperties returns the properties of a usage limit as AWS returns it.
func usageLimitProperties(l *types.UsageLimit) map[string]any {
	properties := map[string]any{}
	for name, value := range map[string]string{
		"ResourceArn":   aws.ToString(l.ResourceArn),
		"UsageType":     string(l.UsageType),
		"Period":        string(l.Period),
		"BreachAction":  string(l.BreachAction),
		"UsageLimitId":  aws.ToString(l.UsageLimitId),
		"UsageLimitArn": aws.ToString(l.UsageLimitArn),
	} {
		if value != "" {
			properties[name] = value
		}
	}
	if l.Amount != nil {
		properties["Amount"] = *l.Amount
	}
	return properties
}

// get returns the usage limit with the identifier, or ErrNotFound. GetUsageLimit answers an ID AWS has no usage limit
// for with ValidationException ("does not exist"), not ResourceNotFoundException, and the ID is its only input, so
// both mean AWS has none.
func (h *UsageLimit) get(ctx context.Context, region string, identifier string) (*types.UsageLimit, error) {
	out, err := h.api.GetUsageLimit(ctx, &redshiftserverless.GetUsageLimitInput{UsageLimitId: aws.String(identifier)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, notFound, validation)
	}
	if out.UsageLimit == nil {
		return nil, native.ErrNotFound
	}
	return out.UsageLimit, nil
}

// Read returns the usage limit's properties.
func (h *UsageLimit) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	limit, err := h.get(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return usageLimitProperties(limit), nil
}

// Create creates the usage limit and returns the ID AWS gives it.
func (h *UsageLimit) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	p := request.Properties
	if err := validateUsageLimit(p); err != nil {
		return "", err
	}
	n, ok := amount(p["Amount"])
	if !ok {
		return "", invalid(usageLimitType, "Amount", "a whole number from 1")
	}
	input := &redshiftserverless.CreateUsageLimitInput{
		ResourceArn: aws.String(awsapi.String(p, "ResourceArn")),
		UsageType:   types.UsageLimitUsageType(awsapi.String(p, "UsageType")),
		Amount:      aws.Int64(n),
	}
	if v := awsapi.String(p, "Period"); v != "" {
		input.Period = types.UsageLimitPeriod(v)
	}
	if v := awsapi.String(p, "BreachAction"); v != "" {
		input.BreachAction = types.UsageLimitBreachAction(v)
	}
	// The call takes no idempotency token and AWS accepts duplicates, so the SDK must not send it again after a lost
	// response: that would make a second limit no record tracks. The outcome is then unknown, and the next deploy finds
	// the limit by its values.
	out, err := h.api.CreateUsageLimit(ctx, input, inRegion(request.Region), noRetry)
	if err != nil {
		return "", rejected("The workgroup "+awsapi.String(p, "ResourceArn"), err)
	}
	if out.UsageLimit == nil || aws.ToString(out.UsageLimit.UsageLimitId) == "" {
		// AWS made the limit but didn't say which: the next deploy finds it by its values.
		return "", &native.Failure{Message: "AWS created the usage limit but returned no UsageLimitId. Deploy again to find it."}
	}
	return aws.ToString(out.UsageLimit.UsageLimitId), nil
}

// Find returns the ID of the usage limit with the values of the create request, AWS's defaults for the ones it leaves
// out, or "" when AWS has none. Limits another alias holds are left out, because AWS accepts identical limits. It fails
// when more than one limit is left, because UCP can't tell which one the lost create made.
func (h *UsageLimit) Find(ctx context.Context, request native.CreateRequest) (string, error) {
	p := request.Properties
	want, ok := amount(p["Amount"])
	if !ok {
		return "", invalid(usageLimitType, "Amount", "a whole number from 1")
	}
	period, action := awsapi.String(p, "Period"), awsapi.String(p, "BreachAction")
	if period == "" {
		period = defaultPeriod
	}
	if action == "" {
		action = defaultBreachAction
	}
	input := &redshiftserverless.ListUsageLimitsInput{
		ResourceArn: aws.String(awsapi.String(p, "ResourceArn")),
		UsageType:   types.UsageLimitUsageType(awsapi.String(p, "UsageType")),
	}
	found := []string{}
	pages := redshiftserverless.NewListUsageLimitsPaginator(h.api, input)
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(request.Region))
		if awsapi.HasCode(err, notFound, validation) {
			// AWS has no workgroup with the ARN, so it has no usage limit of it either.
			return "", nil
		}
		if err != nil {
			return "", err
		}
		for _, l := range page.UsageLimits {
			if aws.ToString(l.ResourceArn) != aws.ToString(input.ResourceArn) || l.UsageType != input.UsageType ||
				aws.ToInt64(l.Amount) != want || string(l.Period) != period || string(l.BreachAction) != action {
				continue
			}
			id := aws.ToString(l.UsageLimitId)
			if request.Held != nil {
				held, err := request.Held(ctx, id)
				if err != nil {
					return "", err
				}
				if held {
					continue
				}
			}
			found = append(found, id)
		}
	}
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		return found[0], nil
	}
	return "", awsapi.Ambiguous("usage limits", found)
}

// Update changes Amount and BreachAction. A template that no longer sets BreachAction sets it back to log, AWS's
// default.
func (h *UsageLimit) Update(ctx context.Context, request native.UpdateRequest) error {
	if err := validateUsageLimit(request.Desired); err != nil {
		return err
	}
	changed := awsapi.Changed(request)
	input := &redshiftserverless.UpdateUsageLimitInput{UsageLimitId: aws.String(request.Identifier)}
	send := false
	// A change AWS already has, such as a removed breach action that is already log, isn't sent, because every
	// change leaves the workgroup busy.
	if slices.Contains(changed, "Amount") {
		n, ok := amount(request.Desired["Amount"])
		if !ok {
			return invalid(usageLimitType, "Amount", "a whole number from 1")
		}
		if current, ok := amount(request.Current["Amount"]); !ok || current != n {
			input.Amount = aws.Int64(n)
			send = true
		}
	}
	if slices.Contains(changed, "BreachAction") {
		action := awsapi.String(request.Desired, "BreachAction")
		if action == "" {
			action = defaultBreachAction
		}
		if action != awsapi.String(request.Current, "BreachAction") {
			input.BreachAction = types.UsageLimitBreachAction(action)
			send = true
		}
	}
	if !send {
		return nil
	}
	_, err := h.api.UpdateUsageLimit(ctx, input, inRegion(request.Region))
	return rejected("The usage limit's workgroup", awsapi.NotFound(err, notFound))
}

// Delete deletes the usage limit.
func (h *UsageLimit) Delete(ctx context.Context, region string, identifier string) error {
	_, err := h.api.DeleteUsageLimit(ctx, &redshiftserverless.DeleteUsageLimitInput{UsageLimitId: aws.String(identifier)}, inRegion(region))
	return rejected("The usage limit's workgroup", awsapi.NotFound(err, notFound))
}

// List returns every usage limit in the region.
func (h *UsageLimit) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	pages := redshiftserverless.NewListUsageLimitsPaginator(h.api, &redshiftserverless.ListUsageLimitsInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return nil, err
		}
		for i := range page.UsageLimits {
			l := &page.UsageLimits[i]
			out = append(out, native.Resource{Identifier: aws.ToString(l.UsageLimitId), Properties: usageLimitProperties(l)})
		}
	}
	return out, nil
}

// Progress reads the usage limit. A create or an update has nothing of its own to wait for, but it changes the
// workgroup, so it waits until the workgroup is free. A delete waits for the limit to be gone.
func (h *UsageLimit) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	limit, err := h.get(ctx, region, identifier)
	if err != nil {
		return native.Progress{}, err
	}
	if op == native.OperationDelete {
		return native.Progress{State: "deleting", Waiting: "deleted"}, nil
	}
	return h.workgroupProgressByArn(ctx, region, aws.ToString(limit.ResourceArn))
}

// workgroupProgressByArn returns the progress of the workgroup with the ARN, which ListWorkgroups finds, because
// GetWorkgroup takes only a name.
func (h *UsageLimit) workgroupProgressByArn(ctx context.Context, region string, arn string) (native.Progress, error) {
	pages := redshiftserverless.NewListWorkgroupsPaginator(h.api, &redshiftserverless.ListWorkgroupsInput{})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, inRegion(region))
		if err != nil {
			return native.Progress{}, err
		}
		for i := range page.Workgroups {
			if aws.ToString(page.Workgroups[i].WorkgroupArn) == arn {
				return statusProgress(&page.Workgroups[i]), nil
			}
		}
	}
	return native.Progress{Done: true}, nil
}
