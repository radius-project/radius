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

// Command gen writes zz_generated_arnproperties.go: for each AWS resource type, the property that holds the ARN of the
// resource itself, or none when the type has no such property.
//
// The inputs are checked in so the output can be checked without AWS access:
//
//   - resource_types.txt is a snapshot of each type's primary identifier and its top-level read-only properties whose
//     names end in "arn". With -refresh, gen rewrites it from the CloudFormation registry in AWS_REGION (default
//     us-west-2), which needs AWS credentials. It lists the public FULLY_MUTABLE AWS types, the same set the AWS
//     Bicep extension is built from.
//   - reviewed.txt holds the decisions for types the rules below can't settle. gen fails when a type needs a decision
//     and has none, or when a decision names a type that no longer exists.
//
// A type's own ARN property is, in order:
//
//  1. The decision in reviewed.txt, when there is one.
//  2. The only candidate among a read-only property named "Arn" or "<ResourceName>Arn", and a single read-only
//     primary identifier whose name ends in "Arn". AWS assigns read-only properties, so these are the resource's own
//     ARN. A primary identifier that isn't read-only is an input, usually the ARN of a parent resource, as in
//     AWS::Lambda::ResourcePolicy, and is never used.
//  3. None, when there is no candidate and no other read-only "*Arn" property. With more than one candidate, or only
//     other read-only "*Arn" properties, the type needs a decision in reviewed.txt.
//
// Usage, from pkg/aws/arnproperties:
//
//	go run ./gen            # render zz_generated_arnproperties.go from the checked-in inputs
//	go run ./gen -refresh   # refresh resource_types.txt from AWS, then render
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/ratelimit"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

const (
	defaultSnapshot = "resource_types.txt"
	defaultReviewed = "reviewed.txt"
	defaultOut      = "zz_generated_arnproperties.go"

	// none marks an empty list in resource_types.txt and "no ARN property" in reviewed.txt.
	none = "none"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err) //nolint:forbidigo // main reports the error and exits.
	}
}

func run() error {
	snapshotPath := flag.String("snapshot", defaultSnapshot, "snapshot of AWS resource types and their ARN candidates")
	reviewedPath := flag.String("reviewed", defaultReviewed, "hand-reviewed ARN property decisions")
	out := flag.String("out", defaultOut, "output Go file")
	refresh := flag.Bool("refresh", false, "rewrite the snapshot from the CloudFormation registry before rendering (needs AWS credentials)")
	flag.Parse()

	if *refresh {
		if err := refreshSnapshot(context.Background(), *snapshotPath); err != nil {
			return err
		}
	}

	src, properties, err := generate(*snapshotPath, *reviewedPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil {
		return err
	}

	withARN := 0
	for _, p := range properties {
		if p != "" {
			withARN++
		}
	}
	fmt.Printf("wrote %d types to %s, %d with an ARN property\n", len(properties), *out, withARN)
	return nil
}

// generate reads the snapshot and the reviewed decisions and renders the generated Go file.
func generate(snapshotPath string, reviewedPath string) ([]byte, map[string]string, error) {
	entries, err := readFile(snapshotPath, parseSnapshot)
	if err != nil {
		return nil, nil, err
	}
	reviewed, err := readFile(reviewedPath, parseReviewed)
	if err != nil {
		return nil, nil, err
	}
	properties, err := arnProperties(entries, reviewed)
	if err != nil {
		return nil, nil, err
	}
	src, err := render(properties)
	return src, properties, err
}

func readFile[T any](path string, parse func(io.Reader) (T, error)) (T, error) {
	f, err := os.Open(path)
	if err != nil {
		var zero T
		return zero, err
	}
	defer f.Close()
	result, err := parse(f)
	if err != nil {
		return result, fmt.Errorf("%s: %w", path, err)
	}
	return result, nil
}

// entry is one AWS resource type from the snapshot.
type entry struct {
	TypeName string
	// PrimaryIdentifier lists the properties that make up the primary identifier.
	PrimaryIdentifier []string
	// ReadOnlyARNs lists the top-level read-only properties whose names end in "arn", case-insensitively.
	ReadOnlyARNs []string
}

// parseSnapshot reads lines of "<type name> <primary identifier properties> <read-only ARN properties or none>", with
// comma-separated lists. Blank lines and lines that start with "#" are ignored.
func parseSnapshot(r io.Reader) ([]entry, error) {
	entries := []entry{}
	err := scanLines(r, func(line int, text string) error {
		fields := strings.Fields(text)
		if len(fields) != 3 {
			return fmt.Errorf("line %d: want \"<type name> <primary identifier> <read-only ARN properties>\", got %q", line, text)
		}
		entries = append(entries, entry{TypeName: fields[0], PrimaryIdentifier: splitList(fields[1]), ReadOnlyARNs: splitList(fields[2])})
		return nil
	})
	return entries, err
}

// parseReviewed reads lines of "<type name> <property or none>", each followed by a "# reason" comment. Blank lines
// and lines that start with "#" are ignored.
func parseReviewed(r io.Reader) (map[string]string, error) {
	reviewed := map[string]string{}
	err := scanLines(r, func(line int, text string) error {
		decision, reason, _ := strings.Cut(text, "#")
		fields := strings.Fields(decision)
		if len(fields) != 2 || strings.TrimSpace(reason) == "" {
			return fmt.Errorf("line %d: want \"<type name> <property or none> # reason\", got %q", line, text)
		}
		if _, ok := reviewed[fields[0]]; ok {
			return fmt.Errorf("line %d: duplicate decision for %s", line, fields[0])
		}
		property := fields[1]
		if property == none {
			property = ""
		}
		reviewed[fields[0]] = property
		return nil
	})
	return reviewed, err
}

func scanLines(r io.Reader, parse func(line int, text string) error) error {
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if err := parse(line, text); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func splitList(field string) []string {
	if field == none {
		return nil
	}
	return strings.Split(field, ",")
}

// arnProperties returns the ARN property of each type, or "" when the type has none. It returns an error listing every
// type that needs a decision in reviewed.txt and every decision that doesn't apply.
func arnProperties(entries []entry, reviewed map[string]string) (map[string]string, error) {
	properties := map[string]string{}
	problems := []string{}
	for _, e := range entries {
		if property, ok := reviewed[e.TypeName]; ok {
			if property != "" && !slices.Contains(e.ReadOnlyARNs, property) {
				problems = append(problems, fmt.Sprintf("%s: reviewed property %q is not one of its read-only ARN properties %v", e.TypeName, property, e.ReadOnlyARNs))
			}
			properties[e.TypeName] = property
			continue
		}

		candidates := ownARNCandidates(e)
		switch {
		case len(candidates) == 1:
			properties[e.TypeName] = candidates[0]
		case len(candidates) > 1:
			problems = append(problems, fmt.Sprintf("%s: needs a decision in %s, more than one candidate: %v", e.TypeName, defaultReviewed, candidates))
		case len(e.ReadOnlyARNs) > 0:
			problems = append(problems, fmt.Sprintf("%s: needs a decision in %s, only other read-only ARN properties: %v", e.TypeName, defaultReviewed, e.ReadOnlyARNs))
		default:
			properties[e.TypeName] = ""
		}
	}

	for typeName := range reviewed {
		if _, ok := properties[typeName]; !ok {
			problems = append(problems, fmt.Sprintf("%s: has a decision in %s but is not in the snapshot", typeName, defaultReviewed))
		}
	}

	if len(problems) > 0 {
		slices.Sort(problems)
		return nil, errors.New(strings.Join(problems, "\n"))
	}
	return properties, nil
}

// ownARNCandidates returns the read-only properties that the naming rules identify as the resource's own ARN.
func ownARNCandidates(e entry) []string {
	parts := strings.Split(e.TypeName, "::")
	resourceName := strings.ToLower(parts[len(parts)-1])

	candidates := []string{}
	for _, p := range e.ReadOnlyARNs {
		lower := strings.ToLower(p)
		if lower == "arn" || lower == resourceName+"arn" {
			candidates = append(candidates, p)
		}
	}
	if len(e.PrimaryIdentifier) == 1 {
		p := e.PrimaryIdentifier[0]
		if strings.HasSuffix(strings.ToLower(p), "arn") && slices.Contains(e.ReadOnlyARNs, p) && !slices.Contains(candidates, p) {
			candidates = append(candidates, p)
		}
	}
	slices.Sort(candidates)
	return candidates
}

func render(properties map[string]string) ([]byte, error) {
	names := []string{}
	for name := range properties {
		names = append(names, name)
	}
	slices.Sort(names)

	var b bytes.Buffer
	b.WriteString(licenseHeader)
	b.WriteString("// Code generated by pkg/aws/arnproperties/gen from " + defaultSnapshot + " and " + defaultReviewed + ". DO NOT EDIT.\n\n")
	b.WriteString("package arnproperties\n\n")
	b.WriteString("// arnProperties maps each known AWS resource type to the property that holds the ARN of the resource itself. The\n")
	b.WriteString("// value is empty when the type has no such property.\n")
	b.WriteString("var arnProperties = map[string]string{\n")
	for _, name := range names {
		fmt.Fprintf(&b, "\t%q: %q,\n", name, properties[name])
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

// refreshSnapshot lists the public FULLY_MUTABLE AWS types in the CloudFormation registry, reads the primary
// identifier and read-only ARN properties of each from its schema, and writes the snapshot to path.
func refreshSnapshot(ctx context.Context, path string) error {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-west-2"
	}
	// The registry throttles DescribeType. Retry throttled calls with backoff instead of failing once the client-side
	// retry quota runs out.
	retryer := func() aws.Retryer {
		return retry.NewStandard(func(o *retry.StandardOptions) {
			o.MaxAttempts = 10
			o.RateLimiter = ratelimit.None
		})
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region), config.WithRetryer(retryer))
	if err != nil {
		return err
	}
	client := cloudformation.NewFromConfig(cfg)

	names := []string{}
	paginator := cloudformation.NewListTypesPaginator(client, &cloudformation.ListTypesInput{
		Type:             types.RegistryTypeResource,
		Visibility:       types.VisibilityPublic,
		ProvisioningType: types.ProvisioningTypeFullyMutable,
		Filters:          &types.TypeFilters{Category: types.CategoryAwsTypes},
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, t := range page.TypeSummaries {
			names = append(names, aws.ToString(t.TypeName))
		}
	}
	slices.Sort(names)

	lines := make([]string, len(names))
	errs := make([]error, len(names))
	work := make(chan int)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			for i := range work {
				e, err := describeEntry(ctx, client, names[i])
				if err != nil {
					errs[i] = fmt.Errorf("%s: %w", names[i], err)
					continue
				}
				lines[i] = formatEntry(e)
			}
		})
	}
	for i := range names {
		work <- i
	}
	close(work)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "# Public FULLY_MUTABLE AWS resource types in the CloudFormation registry (%s, %s): the type name, its primary\n", region, time.Now().UTC().Format("January 2006"))
	b.WriteString("# identifier properties, and its top-level read-only properties whose names end in \"arn\".\n")
	b.WriteString("# Refresh with `go run ./gen -refresh` from pkg/aws/arnproperties.\n")
	for _, line := range lines {
		b.WriteString(line + "\n")
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

func formatEntry(e entry) string {
	readOnly := none
	if len(e.ReadOnlyARNs) > 0 {
		readOnly = strings.Join(e.ReadOnlyARNs, ",")
	}
	return fmt.Sprintf("%s %s %s", e.TypeName, strings.Join(e.PrimaryIdentifier, ","), readOnly)
}

// describeEntry reads the schema of a type.
func describeEntry(ctx context.Context, client *cloudformation.Client, name string) (entry, error) {
	desc, err := client.DescribeType(ctx, &cloudformation.DescribeTypeInput{Type: types.RegistryTypeResource, TypeName: aws.String(name)})
	if err != nil {
		return entry{}, err
	}
	return entryFromSchema(name, []byte(aws.ToString(desc.Schema)))
}

// entryFromSchema reads the primary identifier and the top-level read-only ARN properties from a CloudFormation
// resource schema.
func entryFromSchema(name string, schema []byte) (entry, error) {
	s := struct {
		PrimaryIdentifier  []string `json:"primaryIdentifier"`
		ReadOnlyProperties []string `json:"readOnlyProperties"`
	}{}
	if err := json.Unmarshal(schema, &s); err != nil {
		return entry{}, err
	}

	e := entry{TypeName: name}
	for _, p := range s.PrimaryIdentifier {
		e.PrimaryIdentifier = append(e.PrimaryIdentifier, strings.TrimPrefix(p, "/properties/"))
	}
	if len(e.PrimaryIdentifier) == 0 {
		return entry{}, errors.New("schema has no primary identifier")
	}
	for _, p := range s.ReadOnlyProperties {
		property, ok := strings.CutPrefix(p, "/properties/")
		if ok && !strings.Contains(property, "/") && strings.HasSuffix(strings.ToLower(property), "arn") {
			e.ReadOnlyARNs = append(e.ReadOnlyARNs, property)
		}
	}
	slices.Sort(e.ReadOnlyARNs)
	return e, nil
}

const licenseHeader = `/*
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

`
