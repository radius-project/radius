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

// Command gen writes zz_generated_readonly.go: the public AWS resource types that AWS creates on its own. These types
// are NON_PROVISIONABLE in the CloudFormation registry, and their schema has a read handler but no create handler.
//
// The input is a snapshot of every NON_PROVISIONABLE type and its handlers, kept in non_provisionable_types.txt so the
// output can be checked without AWS access. With -refresh, gen first rewrites the snapshot from the CloudFormation
// registry in AWS_REGION (default us-west-2), which needs AWS credentials.
//
// Usage, from pkg/aws/awstypes:
//
//	go run ./gen            # render zz_generated_readonly.go from the snapshot
//	go run ./gen -refresh   # refresh the snapshot from AWS, then render
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

const (
	defaultSnapshot = "non_provisionable_types.txt"
	defaultOut      = "zz_generated_readonly.go"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, newRegistryClient); err != nil {
		log.Fatal(err) //nolint:forbidigo // main reports the error and exits.
	}
}

// registryClient is the part of the CloudFormation API that -refresh uses.
type registryClient interface {
	cloudformation.ListTypesAPIClient
	DescribeType(ctx context.Context, params *cloudformation.DescribeTypeInput, optFns ...func(*cloudformation.Options)) (*cloudformation.DescribeTypeOutput, error)
}

// newRegistryClient returns a CloudFormation client for region with the default AWS credentials.
func newRegistryClient(ctx context.Context, region string) (registryClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return cloudformation.NewFromConfig(cfg), nil
}

// run parses args, refreshes the snapshot when -refresh is set, and renders the output file.
func run(ctx context.Context, args []string, stdout io.Writer, newClient func(context.Context, string) (registryClient, error)) error {
	flags := flag.NewFlagSet("gen", flag.ContinueOnError)
	snapshotPath := flags.String("snapshot", defaultSnapshot, "snapshot of NON_PROVISIONABLE types and their handlers")
	out := flags.String("out", defaultOut, "output Go file")
	refresh := flags.Bool("refresh", false, "rewrite the snapshot from the CloudFormation registry before rendering (needs AWS credentials)")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if *refresh {
		region := os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-west-2"
		}
		client, err := newClient(ctx, region)
		if err != nil {
			return err
		}
		if err := refreshSnapshot(ctx, client, region, *snapshotPath, time.Now()); err != nil {
			return err
		}
	}

	count, err := generate(*snapshotPath, *out)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %d read-only types to %s\n", count, *out)
	return nil
}

// generate renders the read-only types in the snapshot at snapshotPath to out, and returns how many it wrote.
func generate(snapshotPath string, out string) (int, error) {
	f, err := os.Open(snapshotPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	entries, err := parseSnapshot(f)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", snapshotPath, err)
	}

	names := readOnlyTypes(entries)
	src, err := render(names)
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(out, src, 0o644); err != nil {
		return 0, err
	}
	return len(names), nil
}

// entry is one NON_PROVISIONABLE type and the handlers its CloudFormation schema declares.
type entry struct {
	TypeName string
	Handlers []string
}

// parseSnapshot reads lines of "<type name> <comma-separated handlers or none>". Blank lines and lines that start
// with "#" are ignored.
func parseSnapshot(r io.Reader) ([]entry, error) {
	entries := []entry{}
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		if len(fields) != 2 {
			return nil, fmt.Errorf("line %d: want \"<type name> <handlers>\", got %q", line, text)
		}
		e := entry{TypeName: fields[0]}
		if fields[1] != "none" {
			e.Handlers = strings.Split(fields[1], ",")
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

// readOnlyTypes returns the sorted names of the types that have a read handler and no create handler.
func readOnlyTypes(entries []entry) []string {
	names := []string{}
	for _, e := range entries {
		if slices.Contains(e.Handlers, "read") && !slices.Contains(e.Handlers, "create") {
			names = append(names, e.TypeName)
		}
	}
	slices.Sort(names)
	return names
}

func render(names []string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(licenseHeader)
	b.WriteString("// Code generated by pkg/aws/awstypes/gen from " + defaultSnapshot + ". DO NOT EDIT.\n\n")
	b.WriteString("package awstypes\n\n")
	b.WriteString("// readOnlyTypes are the public AWS resource types that AWS creates on its own. Their schema has a read handler\n")
	b.WriteString("// but no create handler, so Radius can only reference them with `existing`.\n")
	b.WriteString("var readOnlyTypes = []string{\n")
	for _, n := range names {
		fmt.Fprintf(&b, "\t%q,\n", n)
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

// refreshSnapshot lists the public NON_PROVISIONABLE AWS types in the CloudFormation registry, reads the handlers of
// each from its schema, and writes the snapshot to path.
func refreshSnapshot(ctx context.Context, client registryClient, region string, path string, now time.Time) error {
	names := []string{}
	paginator := cloudformation.NewListTypesPaginator(client, &cloudformation.ListTypesInput{
		Type:             types.RegistryTypeResource,
		Visibility:       types.VisibilityPublic,
		ProvisioningType: types.ProvisioningTypeNonProvisionable,
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

	var b bytes.Buffer
	fmt.Fprintf(&b, "# Public NON_PROVISIONABLE AWS resource types in the CloudFormation registry (%s, %s) and the handlers\n", region, now.UTC().Format("January 2006"))
	b.WriteString("# their schema declares. Refresh with `go run ./gen -refresh` from pkg/aws/awstypes.\n")
	for _, name := range names {
		handlers, err := describeHandlers(ctx, client, name)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		list := "none"
		if len(handlers) > 0 {
			list = strings.Join(handlers, ",")
		}
		fmt.Fprintf(&b, "%s %s\n", name, list)
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// maxDescribeAttempts is how many times describeHandlers calls DescribeType before it gives up.
const maxDescribeAttempts = 6

// sleep waits between DescribeType attempts. Tests replace it.
var sleep = time.Sleep

// describeHandlers returns the sorted handler names in the schema of a type, retrying when AWS throttles.
func describeHandlers(ctx context.Context, client registryClient, name string) ([]string, error) {
	var desc *cloudformation.DescribeTypeOutput
	var err error
	for attempt := range maxDescribeAttempts {
		desc, err = client.DescribeType(ctx, &cloudformation.DescribeTypeInput{Type: types.RegistryTypeResource, TypeName: aws.String(name)})
		if err == nil {
			break
		}
		if attempt < maxDescribeAttempts-1 {
			sleep(time.Second << attempt)
		}
	}
	if err != nil {
		return nil, err
	}
	schema := struct {
		Handlers map[string]any `json:"handlers"`
	}{}
	if err := json.Unmarshal([]byte(aws.ToString(desc.Schema)), &schema); err != nil {
		return nil, err
	}
	handlers := []string{}
	for h := range schema.Handlers {
		handlers = append(handlers, h)
	}
	slices.Sort(handlers)
	return handlers, nil
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
