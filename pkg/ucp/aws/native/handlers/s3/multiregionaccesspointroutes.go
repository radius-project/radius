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

package s3

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3control"
	"github.com/aws/aws-sdk-go-v2/service/s3control/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	routesType = "AWS::S3::MultiRegionAccessPointRoutes"

	// activeDial is the TrafficDialPercentage of an active route, which every route of a new Multi-Region Access Point
	// has, and passiveDial that of a passive one. S3 Control rejects every other value.
	activeDial  = 100
	passiveDial = 0

	// noSuchMrapCode is the error S3 Control returns for a Multi-Region Access Point it doesn't have.
	noSuchMrapCode = "NoSuchMultiRegionAccessPoint"

	// defaultControlRegion is the Region the handler sends route calls to when the request's Region doesn't take them.
	defaultControlRegion = "us-east-1"

	// maxMrapLength is the most characters the Mrap parameter takes.
	maxMrapLength = 200
)

// controlRegions are the Regions whose S3 Control endpoints take route calls.
var controlRegions = []string{"us-east-1", "us-west-2", "ap-southeast-2", "ap-northeast-1", "eu-west-1"}

// mrapPattern matches the ARN of a Multi-Region Access Point, with the account ID in the first group.
var mrapPattern = regexp.MustCompile(`^arn:[a-z-]+:s3::([0-9]{12}):accesspoint/[^/]+$`)

var (
	_ native.Handler           = (*MultiRegionAccessPointRoutes)(nil)
	_ native.IdentifierEncoder = (*MultiRegionAccessPointRoutes)(nil)
	_ native.Resetter          = (*MultiRegionAccessPointRoutes)(nil)
)

// MultiRegionAccessPointRoutes serves AWS::S3::MultiRegionAccessPointRoutes, the route configuration of a
// Multi-Region Access Point: whether each of its buckets is active (100) or passive (0). Its identifier is the
// Multi-Region Access Point's ARN, Mrap. The template lists every route, so it gives the whole configuration, which
// one call sets at once, as a failover needs. A template that leaves out a route, or lists one the Multi-Region Access
// Point doesn't have, is rejected before any change. Every Multi-Region Access Point has routes, so UCP takes them
// over and updates them. The type's metadata makes a delete call Reset, which makes every route active again, AWS's
// default.
//
// Route calls go only to the endpoints of five Regions. The handler uses the request's Region when it is one of
// them, and us-east-1 otherwise.
type MultiRegionAccessPointRoutes struct {
	api ControlAPI
}

// NewMultiRegionAccessPointRoutes returns the handler for the routes of a Multi-Region Access Point.
func NewMultiRegionAccessPointRoutes(api ControlAPI) *MultiRegionAccessPointRoutes {
	return &MultiRegionAccessPointRoutes{api: api}
}

// TypeName returns AWS::S3::MultiRegionAccessPointRoutes.
func (h *MultiRegionAccessPointRoutes) TypeName() string {
	return routesType
}

// Options returns how UCP uses the handler. The routes can't be listed.
func (h *MultiRegionAccessPointRoutes) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// controlRegion returns the Region to send route calls to for a request in region.
func controlRegion(region string) string {
	if slices.Contains(controlRegions, region) {
		return region
	}
	return defaultControlRegion
}

// accountOf returns the account ID in the ARN of a Multi-Region Access Point, which route calls send as AccountId.
func accountOf(mrap string) (string, error) {
	match := mrapPattern.FindStringSubmatch(mrap)
	if match == nil || len(mrap) > maxMrapLength {
		return "", rejected("Mrap must be the ARN of a Multi-Region Access Point, such as arn:aws:s3::123456789012:accesspoint/mfzwi23gnjvgw.mrap")
	}
	return match[1], nil
}

// routesOf returns the routes properties list.
func routesOf(properties map[string]any) ([]types.MultiRegionAccessPointRoute, error) {
	var decoded struct {
		Routes []types.MultiRegionAccessPointRoute
	}
	if err := awsapi.Decode(map[string]any{"Routes": properties["Routes"]}, &decoded); err != nil {
		return nil, rejected("each item of Routes must set Bucket or Region as text, and TrafficDialPercentage as a whole number, 0 or 100")
	}
	return decoded.Routes, nil
}

// checkRoutes rejects routes AWS doesn't take: one that names neither a bucket nor a Region, or has no
// TrafficDialPercentage of 0 or 100, and two that name the same bucket or Region.
func checkRoutes(routes []types.MultiRegionAccessPointRoute) error {
	seen := map[string]bool{}
	for _, r := range routes {
		if aws.ToString(r.Bucket) == "" && aws.ToString(r.Region) == "" {
			return rejected("each item of Routes must set Bucket, Region, or both")
		}
		if r.TrafficDialPercentage == nil {
			return rejected("each item of Routes must set TrafficDialPercentage to 0 (passive) or 100 (active)")
		}
		if dial := *r.TrafficDialPercentage; dial != passiveDial && dial != activeDial {
			return rejected("Routes gives the %s the TrafficDialPercentage %d. Use 0 (passive) or 100 (active); S3 takes no other value", describe(r), dial)
		}
		for _, key := range []string{"bucket " + aws.ToString(r.Bucket), "Region " + aws.ToString(r.Region)} {
			if strings.HasSuffix(key, " ") {
				continue
			}
			if seen[key] {
				return rejected("Routes lists %s twice", key)
			}
			seen[key] = true
		}
	}
	return nil
}

// Identifier returns Mrap. An Mrap that isn't the ARN of a Multi-Region Access Point, and Routes AWS doesn't take,
// are rejected before any call.
func (h *MultiRegionAccessPointRoutes) Identifier(properties map[string]any) (string, error) {
	mrap := awsapi.String(properties, "Mrap")
	if mrap == "" {
		return "", nil
	}
	if _, err := accountOf(mrap); err != nil {
		return "", err
	}
	if _, ok := properties["Routes"]; ok {
		routes, err := routesOf(properties)
		if err != nil {
			return "", err
		}
		if err := checkRoutes(routes); err != nil {
			return "", err
		}
	}
	return mrap, nil
}

// get returns the routes of the Multi-Region Access Point mrap, owned by account.
func (h *MultiRegionAccessPointRoutes) get(ctx context.Context, region string, account string, mrap string) ([]types.MultiRegionAccessPointRoute, error) {
	out, err := h.api.GetMultiRegionAccessPointRoutes(ctx, &s3control.GetMultiRegionAccessPointRoutesInput{
		AccountId: aws.String(account),
		Mrap:      aws.String(mrap),
	}, awsapi.InRegion[s3control.Options](controlRegion(region)))
	if err != nil {
		return nil, err
	}
	return out.Routes, nil
}

func (h *MultiRegionAccessPointRoutes) submit(ctx context.Context, region string, account string, mrap string, routes []types.MultiRegionAccessPointRoute) error {
	_, err := h.api.SubmitMultiRegionAccessPointRoutes(ctx, &s3control.SubmitMultiRegionAccessPointRoutesInput{
		AccountId:    aws.String(account),
		Mrap:         aws.String(mrap),
		RouteUpdates: routes,
	}, awsapi.InRegion[s3control.Options](controlRegion(region)))
	return err
}

// Read returns every route of the Multi-Region Access Point, or ErrNotFound when AWS has no such Multi-Region Access
// Point.
func (h *MultiRegionAccessPointRoutes) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	account, err := accountOf(identifier)
	if err != nil {
		return nil, err
	}
	routes, err := h.get(ctx, region, account, identifier)
	if err != nil {
		return nil, notFoundOrRejected(err, noSuchMrapCode)
	}
	list := []any{}
	for _, r := range routes {
		item := map[string]any{}
		if r.Bucket != nil {
			item["Bucket"] = *r.Bucket
		}
		if r.Region != nil {
			item["Region"] = *r.Region
		}
		if r.TrafficDialPercentage != nil {
			item["TrafficDialPercentage"] = int(*r.TrafficDialPercentage)
		}
		list = append(list, item)
	}
	return map[string]any{"Mrap": identifier, "Routes": list}, nil
}

// matches reports whether the template's route names the route AWS has: by Bucket when it sets one, and by Region
// otherwise.
func matches(template types.MultiRegionAccessPointRoute, current types.MultiRegionAccessPointRoute) bool {
	if bucket := aws.ToString(template.Bucket); bucket != "" {
		return bucket == aws.ToString(current.Bucket)
	}
	return aws.ToString(template.Region) == aws.ToString(current.Region)
}

// describe names a route by its bucket, or its Region when it names no bucket.
func describe(r types.MultiRegionAccessPointRoute) string {
	if bucket := aws.ToString(r.Bucket); bucket != "" {
		return "bucket " + bucket
	}
	return "Region " + aws.ToString(r.Region)
}

// apply sends the template's routes in one call, after it checks that they are every route of the Multi-Region
// Access Point: each names one route AWS has, in its Region, and none is left out.
func (h *MultiRegionAccessPointRoutes) apply(ctx context.Context, region string, mrap string, properties map[string]any) error {
	account, err := accountOf(mrap)
	if err != nil {
		return err
	}
	desired, err := routesOf(properties)
	if err != nil {
		return err
	}
	if err := checkRoutes(desired); err != nil {
		return err
	}
	current, err := h.get(ctx, region, account, mrap)
	if err != nil {
		return awsapi.Rejected(err)
	}
	listed := make([]bool, len(current))
	for _, d := range desired {
		i := slices.IndexFunc(current, func(c types.MultiRegionAccessPointRoute) bool { return matches(d, c) })
		switch {
		case i < 0:
			return rejected("Routes lists the %s, which the Multi-Region Access Point doesn't have", describe(d))
		case listed[i]:
			return rejected("Routes lists the route to the %s twice", describe(current[i]))
		case d.Region != nil && aws.ToString(d.Region) != aws.ToString(current[i].Region):
			return rejected("Routes gives the %s the Region %s, but the bucket is in %s", describe(d), aws.ToString(d.Region), aws.ToString(current[i].Region))
		}
		listed[i] = true
	}
	missing := []string{}
	for i, c := range current {
		if !listed[i] {
			missing = append(missing, describe(c))
		}
	}
	if len(missing) > 0 {
		return rejected("Routes leaves out the routes to %s. List every route of the Multi-Region Access Point", strings.Join(missing, ", "))
	}
	return awsapi.Rejected(h.submit(ctx, region, account, mrap, desired))
}

// Create sets the routes of the Multi-Region Access Point and returns its ARN.
func (h *MultiRegionAccessPointRoutes) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	mrap := awsapi.String(request.Properties, "Mrap")
	if err := h.apply(ctx, request.Region, mrap, request.Properties); err != nil {
		return "", err
	}
	return mrap, nil
}

// Update sets the routes of the Multi-Region Access Point to the template's.
func (h *MultiRegionAccessPointRoutes) Update(ctx context.Context, request native.UpdateRequest) error {
	return h.apply(ctx, request.Region, request.Identifier, request.Desired)
}

// Reset makes every route of the Multi-Region Access Point active again, in one call, sending only the routes that
// aren't. It returns ErrNotFound when AWS has no such Multi-Region Access Point.
func (h *MultiRegionAccessPointRoutes) Reset(ctx context.Context, region string, identifier string) error {
	account, err := accountOf(identifier)
	if err != nil {
		return err
	}
	current, err := h.get(ctx, region, account, identifier)
	if err != nil {
		return notFoundOrRejected(err, noSuchMrapCode)
	}
	updates := []types.MultiRegionAccessPointRoute{}
	for _, c := range current {
		if aws.ToInt32(c.TrafficDialPercentage) != activeDial {
			updates = append(updates, types.MultiRegionAccessPointRoute{Bucket: c.Bucket, Region: c.Region, TrafficDialPercentage: aws.Int32(activeDial)})
		}
	}
	if len(updates) == 0 {
		return nil
	}
	return notFoundOrRejected(h.submit(ctx, region, account, identifier, updates), noSuchMrapCode)
}

// Delete resets the routes, as Reset does. The type's metadata makes UCP call Reset.
func (h *MultiRegionAccessPointRoutes) Delete(ctx context.Context, region string, identifier string) error {
	return h.Reset(ctx, region, identifier)
}

// List fails, because the routes can't be listed.
func (h *MultiRegionAccessPointRoutes) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("the AWS type %s can't be listed", routesType)
}

// Progress reports every operation as done, because AWS accepts a route configuration at once. AWS can take up to
// 2 minutes to route requests by it.
func (h *MultiRegionAccessPointRoutes) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	return native.Progress{Done: true}, nil
}
