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

package opensearch

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
)

const (
	// pkgAccount is the fake's caller.
	pkgAccount = "123456789012"

	pkgRegion = "us-west-2"

	// pkgPageSize is how many items a list page has. AWS doesn't document its page size.
	pkgPageSize = 2

	// pkgSettleReads is how many reads of a changing package or association return it in its changing state before
	// it moves on. AWS doesn't say how long an import or an association takes, so the first read never sees it done.
	pkgSettleReads = 2

	// pkgMaxPlugins is the documented limit of plugins a domain can have.
	pkgMaxPlugins = 20

	// pkgValidationCode is the 400 error every package call's page lists, which the fake returns for input that fails
	// a constraint.
	pkgValidationCode = "ValidationException"

	// pkgBaseCode is the error each call lists for any other failure, which the fake returns where the docs state a
	// rule but name no error for it.
	pkgBaseCode = "BaseException"

	// pkgLatestEngine is the latest engine version, which a plugin created without one gets.
	pkgLatestEngine = "OpenSearch_2.17"

	// pkgBucket is the S3 bucket the fake's package files are in.
	pkgBucket = "radius-packages"
)

// pkgDomains are the domains in us-west-2, by name, with their engine versions.
var pkgDomains = map[string]string{
	"logs":    "OpenSearch_2.15",
	"metrics": "OpenSearch_2.17",
	"legacy":  "Elasticsearch_7.10",
}

// pkgObjects are the S3 objects in pkgBucket, by key, with the status their import ends in. Importing any other key
// ends COPY_FAILED.
var pkgObjects = map[string]types.PackageStatus{
	"synonyms.txt":   types.PackageStatusAvailable,
	"synonyms-2.txt": types.PackageStatusAvailable,
	"stopwords.txt":  types.PackageStatusAvailable,
	"plugin.zip":     types.PackageStatusAvailable,
	"license.json":   types.PackageStatusAvailable,
	"bad-plugin.zip": types.PackageStatusValidationFailed,
}

// pkgSeeded are packages that AWS has before any test runs, by ID: the account's own, and an optional plugin AWS owns.
// They aren't in the store, so the Package suite never sees them as its resources.
var pkgSeeded = map[string]types.PackageDetails{
	"F111111111": {
		PackageID: aws.String("F111111111"), PackageName: aws.String("synonyms"), PackageType: types.PackageTypeTxtDictionary,
		PackageOwner: aws.String(pkgAccount), PackageStatus: types.PackageStatusAvailable, AvailablePackageVersion: aws.String("v1"),
	},
	"F222222222": {
		PackageID: aws.String("F222222222"), PackageName: aws.String("stopwords"), PackageType: types.PackageTypeTxtDictionary,
		PackageOwner: aws.String(pkgAccount), PackageStatus: types.PackageStatusAvailable, AvailablePackageVersion: aws.String("v1"),
	},
	"pkg-0a1b2c3d": {
		PackageID: aws.String("pkg-0a1b2c3d"), PackageName: aws.String("custom-plugin"), PackageType: types.PackageTypeZipPlugin,
		EngineVersion: aws.String("OpenSearch_2.15"), PackageOwner: aws.String(pkgAccount), PackageStatus: types.PackageStatusAvailable,
		AvailablePackageVersion: aws.String("v1"),
	},
	"pkg-1c2d3e4f": {
		PackageID: aws.String("pkg-1c2d3e4f"), PackageName: aws.String("custom-plugin-license"), PackageType: types.PackageTypePackageLicense,
		PackageOwner: aws.String(pkgAccount), PackageStatus: types.PackageStatusAvailable, AvailablePackageVersion: aws.String("v1"),
	},
	"G333333333": {
		PackageID: aws.String("G333333333"), PackageName: aws.String("analysis-icu"), PackageType: types.PackageTypeZipPlugin,
		EngineVersion: aws.String("OpenSearch_2.15"), PackageOwner: aws.String("AWS"), PackageStatus: types.PackageStatusAvailable,
		AvailablePackageVersion: aws.String("v1"),
	},
}

var (
	pkgFilterValuePattern = regexp.MustCompile(`^[0-9a-zA-Z\*\.\_\\\/\?-]+$`)
	pkgEpoch              = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

// pkgChange is a change AWS is making to a package or an association: the status it reports, how many reads it
// reports it for, and what follows.
type pkgChange struct {
	status types.PackageStatus
	reads  int
	// lag is how many reads still report the package as it was, before the change shows.
	lag int
	// next is the status after this one: another changing status, an end status, or "" when the change removes the
	// package.
	next []types.PackageStatus
	// version is the version the package has once the change ends AVAILABLE, or "" to keep its version, and commit
	// the commit message of the update that made it.
	version string
	commit  *string
}

// pkgFake is an in-memory Amazon OpenSearch Service API for packages and package associations. It keeps packages and
// associations in an awsfake store, in their types' property format, so the suite's Cloud Control sees the same
// resources, and tolerates records the suite writes there directly. It follows the rules in packageAPI and
// associationAPI, and where AWS's docs are silent it takes the pessimistic side:
//
//   - A created package reports COPYING, then VALIDATING, for pkgSettleReads reads each before it ends AVAILABLE, or
//     COPY_FAILED or VALIDATION_FAILED. An updated package keeps reporting AVAILABLE with its old version for one more
//     read before it goes through the same steps to a new version. A deleted package reports DELETING, then DELETED,
//     before it is gone.
//   - An association reports ASSOCIATING or DISSOCIATING for pkgSettleReads reads. While any association of a domain
//     is changing, or the domain is processing a change of its own, AWS rejects an association or dissociation on
//     that domain with ConflictException.
//   - A package that is still importing or deleting can't be updated or deleted, and one that isn't AVAILABLE can't be
//     associated.
type pkgFake struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// changes holds each package's change while AWS is making one, by package ID, and failed holds the error details
	// of packages whose import failed.
	changes map[string]*pkgChange
	failed  map[string]*types.ErrorDetails

	// deleted holds packages AWS reports DELETED, by ID, with how many more reads report them.
	deleted map[string]*pkgDeleted

	// history holds each package's versions, which AWS lists once an import ends AVAILABLE.
	history map[string][]types.PackageVersionHistory

	// versions and updated hold each package's version number and last update time.
	versions map[string]int
	updated  map[string]time.Time
	clock    time.Time
	nextID   int

	// assocChanges holds each association's change, by DomainName|PackageID: its status and how many more reads
	// report it.
	assocChanges map[string]*pkgAssocChange

	// processing is how many more calls each domain stays processing a change of its own, such as an update outside
	// Radius.
	processing map[string]int

	// failAssociate and failDissociate make the next association or dissociation end ASSOCIATION_FAILED or
	// DISSOCIATION_FAILED, as when an index still uses the package.
	failAssociate, failDissociate bool

	// calls counts the calls that reached the fake, by method.
	calls map[string]int

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

// pkgDeleted is a package AWS reports DELETED for reads more reads.
type pkgDeleted struct {
	details types.PackageDetails
	reads   int
}

// pkgAssocChange is an association AWS is changing: the status it reports, for reads more reads, and the status it
// ends in, or "" when it ends dissociated.
type pkgAssocChange struct {
	status types.DomainPackageStatus
	reads  int
	end    types.DomainPackageStatus
}

var (
	_ PackageAPI        = (*pkgFake)(nil)
	_ PackageAccountAPI = (*pkgFake)(nil)
)

func newPkgFake(store *awsfake.CloudControl) *pkgFake {
	return &pkgFake{
		store:        store,
		changes:      map[string]*pkgChange{},
		failed:       map[string]*types.ErrorDetails{},
		deleted:      map[string]*pkgDeleted{},
		versions:     map[string]int{},
		history:      map[string][]types.PackageVersionHistory{},
		updated:      map[string]time.Time{},
		clock:        pkgEpoch,
		assocChanges: map[string]*pkgAssocChange{},
		processing:   map[string]int{},
		calls:        map[string]int{},
		errs:         map[string]error{},
	}
}

func pkgFault(code string, format string, args ...any) error {
	return &smithy.GenericAPIError{Code: code, Message: fmt.Sprintf(format, args...), Fault: smithy.FaultClient}
}

// begin counts a call, passes time, and returns the error errs holds for it.
func (f *pkgFake) begin(method string) error {
	f.calls[method]++
	f.clock = f.clock.Add(time.Second)
	for name, n := range f.processing {
		if n <= 1 {
			delete(f.processing, name)
		} else {
			f.processing[name] = n - 1
		}
	}
	return f.errs[method]
}

// statusOf returns the package's status: AVAILABLE unless the fake keeps another for it.
func (f *pkgFake) statusOf(id string) types.PackageStatus {
	if c, ok := f.changes[id]; ok && c.lag == 0 {
		return c.status
	}
	if _, ok := f.failed[id]; ok {
		return f.failedStatus(id)
	}
	return types.PackageStatusAvailable
}

func (f *pkgFake) failedStatus(id string) types.PackageStatus {
	if f.failed[id] != nil && aws.ToString(f.failed[id].ErrorType) == "ValidationFailed" {
		return types.PackageStatusValidationFailed
	}
	return types.PackageStatusCopyFailed
}

// lookup returns the package with the ID, from the store or the seeded packages, or nil when AWS has none.
func (f *pkgFake) lookup(id string) *types.PackageDetails {
	if p := f.store.Resource(packageType, id); p != nil {
		return f.details(id, p)
	}
	if p, ok := pkgSeeded[id]; ok {
		return &p
	}
	return nil
}

// details returns the stored package as AWS describes it.
func (f *pkgFake) details(id string, p map[string]any) *types.PackageDetails {
	str := func(m map[string]any, name string) *string {
		if v, ok := m[name].(string); ok {
			return aws.String(v)
		}
		return nil
	}
	d := &types.PackageDetails{
		PackageID:          aws.String(id),
		PackageName:        str(p, "PackageName"),
		PackageType:        types.PackageType(aws.ToString(str(p, "PackageType"))),
		EngineVersion:      str(p, "EngineVersion"),
		PackageDescription: str(p, "PackageDescription"),
		PackageOwner:       str(p, "PackageOwner"),
		PackageStatus:      f.statusOf(id),
		LastUpdatedAt:      aws.Time(f.updated[id]),
	}
	if v := f.versions[id]; v > 0 {
		d.AvailablePackageVersion = aws.String(fmt.Sprintf("v%d", v))
	}
	if o, ok := p["PackageEncryptionOptions"].(map[string]any); ok {
		enabled, _ := o["EncryptionEnabled"].(bool)
		d.PackageEncryptionOptions = &types.PackageEncryptionOptions{EncryptionEnabled: aws.Bool(enabled), KmsKeyIdentifier: str(o, "KmsKeyIdentifier")}
	}
	if o, ok := p["PackageVendingOptions"].(map[string]any); ok {
		enabled, _ := o["VendingEnabled"].(bool)
		d.PackageVendingOptions = &types.PackageVendingOptions{VendingEnabled: aws.Bool(enabled)}
	}
	if d.PackageStatus == types.PackageStatusCopyFailed || d.PackageStatus == types.PackageStatusValidationFailed {
		d.ErrorDetails = f.failed[id]
	}
	return d
}

// settle moves the change of a package a read returned, once it was read pkgSettleReads times in its status.
func (f *pkgFake) settle(id string) {
	c, ok := f.changes[id]
	if !ok {
		return
	}
	if c.lag > 0 {
		c.lag--
		return
	}
	if c.reads++; c.reads < pkgSettleReads {
		return
	}
	c.reads = 0
	next := c.next[0]
	c.next = c.next[1:]
	switch next {
	case "":
		delete(f.changes, id)
		if p := f.lookup(id); p != nil {
			p.PackageStatus = types.PackageStatusDeleted
			f.deleted[id] = &pkgDeleted{details: *p, reads: pkgSettleReads}
		}
		f.store.Remove(packageType, id)
		delete(f.failed, id)
	case types.PackageStatusAvailable:
		delete(f.changes, id)
		delete(f.failed, id)
		if c.version != "" {
			f.versions[id], _ = strconv.Atoi(strings.TrimPrefix(c.version, "v"))
			f.history[id] = append(f.history[id], types.PackageVersionHistory{
				PackageVersion: aws.String(c.version), CommitMessage: c.commit, CreatedAt: aws.Time(f.clock),
			})
		}
		f.updated[id] = f.clock
	case types.PackageStatusCopyFailed, types.PackageStatusValidationFailed:
		delete(f.changes, id)
		f.failed[id] = &types.ErrorDetails{ErrorType: aws.String("CopyFailed"), ErrorMessage: aws.String("AWS couldn't copy the package from S3")}
		if next == types.PackageStatusValidationFailed {
			f.failed[id] = &types.ErrorDetails{ErrorType: aws.String("ValidationFailed"), ErrorMessage: aws.String("The provided plugin could not be loaded")}
		}
		f.updated[id] = f.clock
	default:
		c.status = next
	}
}

// importSteps returns the statuses an import of the S3 object goes through after COPYING.
func importSteps(source *types.PackageSource) []types.PackageStatus {
	end, ok := pkgObjects[aws.ToString(source.S3Key)]
	if aws.ToString(source.S3BucketName) != pkgBucket || !ok {
		return []types.PackageStatus{types.PackageStatusCopyFailed}
	}
	return []types.PackageStatus{types.PackageStatusValidating, end}
}

// checkSource checks a PackageSource against its documented lengths.
func checkSource(source *types.PackageSource) error {
	if source == nil {
		return pkgFault(pkgValidationCode, "PackageSource is required")
	}
	if b := aws.ToString(source.S3BucketName); len(b) < 3 || len(b) > 63 {
		return pkgFault(pkgValidationCode, "S3BucketName must be 3 to 63 characters")
	}
	if k := aws.ToString(source.S3Key); k == "" || len(k) > 1024 {
		return pkgFault(pkgValidationCode, "S3Key must be 1 to 1024 characters")
	}
	return nil
}

// checkOptions checks the configuration and encryption options of a create or update against their documented
// constraints.
func checkOptions(config *types.PackageConfiguration, encryption *types.PackageEncryptionOptions, description *string) error {
	if len(aws.ToString(description)) > 1024 {
		return pkgFault(pkgValidationCode, "PackageDescription must be at most 1024 characters")
	}
	if c := config; c != nil {
		levels := types.RequirementLevel("").Values()
		if !slices.Contains(levels, c.ConfigurationRequirement) || !slices.Contains(levels, c.LicenseRequirement) {
			return pkgFault(pkgValidationCode, "ConfigurationRequirement and LicenseRequirement are required")
		}
		if path := aws.ToString(c.LicenseFilepath); c.LicenseFilepath != nil && (len(path) > 256 || strings.Contains(path, "..") || !licenseFilepathPattern.MatchString(path)) {
			return pkgFault(pkgValidationCode, "LicenseFilepath doesn't match the pattern")
		}
	}
	if o := encryption; o != nil {
		if o.EncryptionEnabled == nil {
			return pkgFault(pkgValidationCode, "EncryptionEnabled is required")
		}
		if key := aws.ToString(o.KmsKeyIdentifier); o.KmsKeyIdentifier != nil && (key == "" || len(key) > 500) {
			return pkgFault(pkgValidationCode, "KmsKeyIdentifier must be 1 to 500 characters")
		}
	}
	return nil
}

// encryptionProperties returns encryption options in the type's property format.
func encryptionProperties(o *types.PackageEncryptionOptions) map[string]any {
	p := map[string]any{"EncryptionEnabled": aws.ToBool(o.EncryptionEnabled)}
	if o.KmsKeyIdentifier != nil {
		p["KmsKeyIdentifier"] = *o.KmsKeyIdentifier
	}
	return p
}

// sourceProperties returns a PackageSource in the type's property format.
func sourceProperties(s *types.PackageSource) map[string]any {
	return map[string]any{"S3BucketName": aws.ToString(s.S3BucketName), "S3Key": aws.ToString(s.S3Key)}
}

// configurationProperties returns a PackageConfiguration in the type's property format.
func configurationProperties(c *types.PackageConfiguration) map[string]any {
	p := map[string]any{"ConfigurationRequirement": string(c.ConfigurationRequirement), "LicenseRequirement": string(c.LicenseRequirement)}
	if c.LicenseFilepath != nil {
		p["LicenseFilepath"] = *c.LicenseFilepath
	}
	if c.RequiresRestartForConfigurationUpdate != nil {
		p["RequiresRestartForConfigurationUpdate"] = *c.RequiresRestartForConfigurationUpdate
	}
	return p
}

// packageIDs returns the IDs of the packages AWS describes: the store's, the seeded ones, and the deleted ones it still
// reports, sorted.
func (f *pkgFake) packageIDs() []string {
	ids := f.store.List(packageType)
	for id := range pkgSeeded {
		ids = append(ids, id)
	}
	for id := range f.deleted {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

func (f *pkgFake) CreatePackage(ctx context.Context, in *opensearch.CreatePackageInput, _ ...func(*opensearch.Options)) (*opensearch.CreatePackageOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreatePackage"); err != nil {
		return nil, err
	}
	name := aws.ToString(in.PackageName)
	if len(name) < 3 || len(name) > 256 || !packageNamePattern.MatchString(name) {
		return nil, pkgFault(pkgValidationCode, "PackageName %q doesn't match the pattern", name)
	}
	if !slices.Contains(types.PackageType("").Values(), in.PackageType) {
		return nil, pkgFault(pkgValidationCode, "PackageType %q isn't a valid value", in.PackageType)
	}
	if err := checkSource(in.PackageSource); err != nil {
		return nil, err
	}
	if in.EngineVersion != nil {
		if in.PackageType != types.PackageTypeZipPlugin {
			return nil, pkgFault(pkgValidationCode, "EngineVersion can only be set for a ZIP-PLUGIN package")
		}
		if !engineVersionPattern.MatchString(*in.EngineVersion) {
			return nil, pkgFault(pkgValidationCode, "EngineVersion %q doesn't match the pattern", *in.EngineVersion)
		}
	}
	if err := checkOptions(in.PackageConfiguration, in.PackageEncryptionOptions, in.PackageDescription); err != nil {
		return nil, err
	}
	if o := in.PackageVendingOptions; o != nil && o.VendingEnabled == nil {
		return nil, pkgFault(pkgValidationCode, "VendingEnabled is required")
	}
	engine := aws.ToString(in.EngineVersion)
	if engine == "" && in.PackageType == types.PackageTypeZipPlugin {
		// The docs say EngineVersion defaults to the latest version of OpenSearch.
		engine = pkgLatestEngine
	}
	for _, id := range f.packageIDs() {
		p := f.lookup(id)
		if p == nil || aws.ToString(p.PackageName) != name || aws.ToString(p.PackageOwner) != pkgAccount {
			continue
		}
		if p.PackageType == types.PackageTypeZipPlugin && in.PackageType == types.PackageTypeZipPlugin && aws.ToString(p.EngineVersion) != engine {
			continue
		}
		return nil, pkgFault("ResourceAlreadyExistsException", "a package named %q already exists", name)
	}
	f.nextID++
	id := fmt.Sprintf("pkg-%08x", f.nextID)
	if in.PackageType == types.PackageTypeTxtDictionary {
		id = fmt.Sprintf("F%09d", f.nextID)
	}
	properties := map[string]any{"PackageID": id, "PackageName": name, "PackageType": string(in.PackageType), "PackageOwner": pkgAccount}
	if engine != "" {
		properties["EngineVersion"] = engine
	}
	if in.PackageDescription != nil {
		properties["PackageDescription"] = *in.PackageDescription
	}
	if in.PackageEncryptionOptions != nil {
		properties["PackageEncryptionOptions"] = encryptionProperties(in.PackageEncryptionOptions)
	}
	if in.PackageVendingOptions != nil {
		properties["PackageVendingOptions"] = map[string]any{"VendingEnabled": aws.ToBool(in.PackageVendingOptions.VendingEnabled)}
	}
	// AWS doesn't return the source or the configuration as sent, but the store keeps them, so the suite can check
	// that AWS has the template's write-only values.
	properties["PackageSource"] = sourceProperties(in.PackageSource)
	if c := in.PackageConfiguration; c != nil {
		properties["PackageConfiguration"] = configurationProperties(c)
	}
	f.store.Put(packageType, id, properties)
	f.updated[id] = f.clock
	f.changes[id] = &pkgChange{status: types.PackageStatusCopying, next: importSteps(in.PackageSource), version: "v1"}
	return &opensearch.CreatePackageOutput{PackageDetails: f.lookup(id)}, nil
}

// filterMatches reports whether the package has one of values for the filter's field.
func filterMatches(p *types.PackageDetails, filter types.DescribePackagesFilter) bool {
	field := map[types.DescribePackagesFilterName]string{
		types.DescribePackagesFilterNamePackageID:     aws.ToString(p.PackageID),
		types.DescribePackagesFilterNamePackageName:   aws.ToString(p.PackageName),
		types.DescribePackagesFilterNamePackageStatus: string(p.PackageStatus),
		types.DescribePackagesFilterNamePackageType:   string(p.PackageType),
		types.DescribePackagesFilterNameEngineVersion: aws.ToString(p.EngineVersion),
		types.DescribePackagesFilterNamePackageOwner:  aws.ToString(p.PackageOwner),
	}[filter.Name]
	return slices.Contains(filter.Value, field)
}

func (f *pkgFake) DescribePackages(ctx context.Context, in *opensearch.DescribePackagesInput, _ ...func(*opensearch.Options)) (*opensearch.DescribePackagesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribePackages"); err != nil {
		return nil, err
	}
	if in.MaxResults > 100 {
		return nil, pkgFault(pkgValidationCode, "MaxResults must be at most 100")
	}
	for _, filter := range in.Filters {
		if !slices.Contains(filter.Name.Values(), filter.Name) {
			return nil, pkgFault(pkgValidationCode, "filter name %q isn't a valid value", filter.Name)
		}
		if len(filter.Value) == 0 {
			return nil, pkgFault(pkgValidationCode, "a filter needs at least one value")
		}
		for _, v := range filter.Value {
			if !pkgFilterValuePattern.MatchString(v) {
				return nil, pkgFault(pkgValidationCode, "filter value %q doesn't match the pattern", v)
			}
		}
	}
	list := []types.PackageDetails{}
	for _, id := range f.packageIDs() {
		var p *types.PackageDetails
		if d, ok := f.deleted[id]; ok && f.store.Resource(packageType, id) == nil {
			p = &d.details
		} else {
			p = f.lookup(id)
		}
		if p == nil || slices.ContainsFunc(in.Filters, func(filter types.DescribePackagesFilter) bool { return !filterMatches(p, filter) }) {
			continue
		}
		list = append(list, *p)
	}
	items, next := pkgPage(list, in.NextToken)
	for _, p := range items {
		id := aws.ToString(p.PackageID)
		if d, ok := f.deleted[id]; ok {
			if d.reads--; d.reads <= 0 {
				delete(f.deleted, id)
			}
		}
		f.settle(id)
	}
	return &opensearch.DescribePackagesOutput{PackageDetailsList: items, NextToken: next}, nil
}

func (f *pkgFake) UpdatePackage(ctx context.Context, in *opensearch.UpdatePackageInput, _ ...func(*opensearch.Options)) (*opensearch.UpdatePackageOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("UpdatePackage"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.PackageID)
	if !packageIDPattern.MatchString(id) {
		return nil, pkgFault(pkgValidationCode, "PackageID %q doesn't match the pattern", id)
	}
	if err := checkSource(in.PackageSource); err != nil {
		return nil, err
	}
	if len(aws.ToString(in.CommitMessage)) > 160 {
		return nil, pkgFault(pkgValidationCode, "CommitMessage must be at most 160 characters")
	}
	if err := checkOptions(in.PackageConfiguration, in.PackageEncryptionOptions, in.PackageDescription); err != nil {
		return nil, err
	}
	p := f.store.Resource(packageType, id)
	if p == nil {
		return nil, pkgFault("ResourceNotFoundException", "the package %q doesn't exist", id)
	}
	if status := f.statusOf(id); status != types.PackageStatusAvailable {
		return nil, pkgFault(pkgBaseCode, "the package %q is %s", id, status)
	}
	if in.PackageDescription != nil {
		p["PackageDescription"] = *in.PackageDescription
	}
	if in.PackageEncryptionOptions != nil {
		p["PackageEncryptionOptions"] = encryptionProperties(in.PackageEncryptionOptions)
	}
	if in.PackageConfiguration != nil {
		p["PackageConfiguration"] = configurationProperties(in.PackageConfiguration)
	}
	p["PackageSource"] = sourceProperties(in.PackageSource)
	f.store.Put(packageType, id, p)
	f.changes[id] = &pkgChange{
		status:  types.PackageStatusCopying,
		lag:     1,
		next:    importSteps(in.PackageSource),
		version: fmt.Sprintf("v%d", f.versions[id]+1),
		commit:  in.CommitMessage,
	}
	return &opensearch.UpdatePackageOutput{PackageDetails: f.lookup(id)}, nil
}

// associated reports whether any domain has the package.
func (f *pkgFake) associated(id string) bool {
	for _, key := range f.store.List(packageAssociationType) {
		if strings.HasSuffix(key, "|"+id) {
			return true
		}
	}
	return false
}

func (f *pkgFake) DeletePackage(ctx context.Context, in *opensearch.DeletePackageInput, _ ...func(*opensearch.Options)) (*opensearch.DeletePackageOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeletePackage"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.PackageID)
	if !packageIDPattern.MatchString(id) {
		return nil, pkgFault(pkgValidationCode, "PackageID %q doesn't match the pattern", id)
	}
	if f.store.Resource(packageType, id) == nil {
		return nil, pkgFault("ResourceNotFoundException", "the package %q doesn't exist", id)
	}
	if f.associated(id) {
		return nil, pkgFault("ConflictException", "the package %q is associated with a domain", id)
	}
	if status := f.statusOf(id); status == types.PackageStatusCopying || status == types.PackageStatusValidating || status == types.PackageStatusDeleting {
		return nil, pkgFault(pkgBaseCode, "the package %q is %s", id, status)
	}
	f.changes[id] = &pkgChange{status: types.PackageStatusDeleting, next: []types.PackageStatus{""}}
	return &opensearch.DeletePackageOutput{PackageDetails: f.lookup(id)}, nil
}

// pkgPage returns the items of the page that token starts, and the next page's token, or nil on the last page.
func pkgPage[T any](items []T, token *string) ([]T, *string) {
	start, _ := strconv.Atoi(aws.ToString(token))
	end := min(start+pkgPageSize, len(items))
	var next *string
	if end < len(items) {
		next = aws.String(strconv.Itoa(end))
	}
	return items[min(start, end):end], next
}

// checkDomain returns ValidationException for a name outside the documented length and pattern, and
// ResourceNotFoundException for a domain the fake doesn't have.
func checkDomain(name string) error {
	if !validDomainName(name) {
		return pkgFault(pkgValidationCode, "DomainName %q doesn't match the pattern", name)
	}
	if _, ok := pkgDomains[name]; !ok {
		return pkgFault("ResourceNotFoundException", "the domain %q doesn't exist", name)
	}
	return nil
}

// assocStatus returns the association's status: ACTIVE unless the fake keeps another for it.
func (f *pkgFake) assocStatus(key string) types.DomainPackageStatus {
	if c, ok := f.assocChanges[key]; ok {
		return c.status
	}
	return types.DomainPackageStatusActive
}

// domainBusy reports whether the domain is processing a change, or AWS is changing one of its associations.
func (f *pkgFake) domainBusy(domain string) bool {
	if f.processing[domain] > 0 {
		return true
	}
	for _, key := range f.associations(domain) {
		if s := f.assocStatus(key); s == types.DomainPackageStatusAssociating || s == types.DomainPackageStatusDissociating {
			return true
		}
	}
	return false
}

// associations returns the keys of the domain's associations, in the order the store lists them.
func (f *pkgFake) associations(domain string) []string {
	out := []string{}
	for _, key := range f.store.List(packageAssociationType) {
		if strings.HasPrefix(key, domain+"|") {
			out = append(out, key)
		}
	}
	return out
}

// assocDetails returns the stored association as AWS lists it.
func (f *pkgFake) assocDetails(key string) types.DomainPackageDetails {
	p := f.store.Resource(packageAssociationType, key)
	domain, id, _ := strings.Cut(key, "|")
	str := func(name string) *string {
		if v, ok := p[name].(string); ok {
			return aws.String(v)
		}
		return nil
	}
	d := types.DomainPackageDetails{
		DomainName:          aws.String(domain),
		PackageID:           aws.String(id),
		PackageName:         str("PackageName"),
		PackageType:         types.PackageType(aws.ToString(str("PackageType"))),
		PackageVersion:      str("PackageVersion"),
		ReferencePath:       str("ReferencePath"),
		DomainPackageStatus: f.assocStatus(key),
	}
	if list, ok := p["PrerequisitePackageIDList"].([]any); ok {
		for _, v := range list {
			d.PrerequisitePackageIDList = append(d.PrerequisitePackageIDList, fmt.Sprint(v))
		}
	}
	if c, ok := p["AssociationConfiguration"].(map[string]any); ok {
		d.AssociationConfiguration = &types.PackageAssociationConfiguration{}
		if o, ok := c["KeyStoreAccessOption"].(map[string]any); ok {
			enabled, _ := o["KeyStoreAccessEnabled"].(bool)
			d.AssociationConfiguration.KeyStoreAccessOption = &types.KeyStoreAccessOption{KeyStoreAccessEnabled: aws.Bool(enabled)}
			if arn, ok := o["KeyAccessRoleArn"].(string); ok {
				d.AssociationConfiguration.KeyStoreAccessOption.KeyAccessRoleArn = aws.String(arn)
			}
		}
	}
	if s := d.DomainPackageStatus; s == types.DomainPackageStatusAssociationFailed || s == types.DomainPackageStatusDissociationFailed {
		d.ErrorDetails = &types.ErrorDetails{ErrorType: aws.String(string(s)), ErrorMessage: aws.String("an index uses the package")}
	}
	return d
}

// settleAssoc moves the change of an association a read returned, once it was read pkgSettleReads times.
func (f *pkgFake) settleAssoc(key string) {
	c, ok := f.assocChanges[key]
	if !ok || (c.status != types.DomainPackageStatusAssociating && c.status != types.DomainPackageStatusDissociating) {
		return
	}
	if c.reads++; c.reads < pkgSettleReads {
		return
	}
	switch c.end {
	case types.DomainPackageStatusActive:
		delete(f.assocChanges, key)
	case "":
		delete(f.assocChanges, key)
		f.store.Remove(packageAssociationType, key)
		// The docs say dissociating a plugin also removes its configuration and license packages.
		domain, id, _ := strings.Cut(key, "|")
		for _, other := range f.associations(domain) {
			if slices.Contains(f.assocDetails(other).PrerequisitePackageIDList, id) {
				delete(f.assocChanges, other)
				f.store.Remove(packageAssociationType, other)
			}
		}
	default:
		c.status, c.reads = c.end, 0
	}
}

// checkAssociation checks the domain, package, and options of an AssociatePackage against their documented
// constraints.
func checkAssociation(in *opensearch.AssociatePackageInput) error {
	if !packageIDPattern.MatchString(aws.ToString(in.PackageID)) {
		return pkgFault(pkgValidationCode, "PackageID %q doesn't match the pattern", aws.ToString(in.PackageID))
	}
	for _, p := range in.PrerequisitePackageIDList {
		if !packageIDPattern.MatchString(p) {
			return pkgFault(pkgValidationCode, "prerequisite %q doesn't match the pattern", p)
		}
	}
	if c := in.AssociationConfiguration; c != nil && c.KeyStoreAccessOption != nil {
		o := c.KeyStoreAccessOption
		if o.KeyStoreAccessEnabled == nil {
			return pkgFault(pkgValidationCode, "KeyStoreAccessEnabled is required")
		}
		if arn := aws.ToString(o.KeyAccessRoleArn); o.KeyAccessRoleArn != nil && (len(arn) < 20 || len(arn) > 2048 || !keyAccessRolePattern.MatchString(arn)) {
			return pkgFault(pkgValidationCode, "KeyAccessRoleArn doesn't match the pattern")
		}
	}
	return checkDomain(aws.ToString(in.DomainName))
}

func (f *pkgFake) AssociatePackage(ctx context.Context, in *opensearch.AssociatePackageInput, _ ...func(*opensearch.Options)) (*opensearch.AssociatePackageOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("AssociatePackage"); err != nil {
		return nil, err
	}
	if err := checkAssociation(in); err != nil {
		return nil, err
	}
	domain, id := aws.ToString(in.DomainName), aws.ToString(in.PackageID)
	p := f.lookup(id)
	if p == nil {
		return nil, pkgFault("ResourceNotFoundException", "the package %q doesn't exist", id)
	}
	if f.domainBusy(domain) {
		return nil, pkgFault("ConflictException", "the domain %q is processing a change", domain)
	}
	if p.PackageStatus != types.PackageStatusAvailable {
		return nil, pkgFault(pkgBaseCode, "the package %q is %s", id, p.PackageStatus)
	}
	if p.PackageType == types.PackageTypeZipPlugin && aws.ToString(p.EngineVersion) != pkgDomains[domain] {
		return nil, pkgFault(pkgBaseCode, "the plugin %q is for %s, and the domain %q runs %s", id, aws.ToString(p.EngineVersion), domain, pkgDomains[domain])
	}
	for _, pre := range in.PrerequisitePackageIDList {
		key := domain + "|" + pre
		if f.store.Resource(packageAssociationType, key) == nil || f.assocStatus(key) != types.DomainPackageStatusActive {
			return nil, pkgFault(pkgBaseCode, "the prerequisite %q isn't associated with the domain %q", pre, domain)
		}
	}
	key := domain + "|" + id
	if p.PackageType == types.PackageTypeZipPlugin && f.store.Resource(packageAssociationType, key) == nil {
		plugins := 0
		for _, other := range f.associations(domain) {
			if f.assocDetails(other).PackageType == types.PackageTypeZipPlugin {
				plugins++
			}
		}
		if plugins >= pkgMaxPlugins {
			return nil, pkgFault(pkgBaseCode, "a domain can have at most %d plugins", pkgMaxPlugins)
		}
	}
	properties := map[string]any{
		"DomainName":     domain,
		"PackageID":      id,
		"PackageName":    aws.ToString(p.PackageName),
		"PackageType":    string(p.PackageType),
		"PackageVersion": aws.ToString(p.AvailablePackageVersion),
	}
	if p.PackageType == types.PackageTypeTxtDictionary {
		properties["ReferencePath"] = "analyzers/" + id
	}
	if in.PrerequisitePackageIDList != nil {
		properties["PrerequisitePackageIDList"] = anyStrings(in.PrerequisitePackageIDList)
	}
	if c := in.AssociationConfiguration; c != nil {
		config := map[string]any{}
		if c.KeyStoreAccessOption != nil {
			o := map[string]any{"KeyStoreAccessEnabled": aws.ToBool(c.KeyStoreAccessOption.KeyStoreAccessEnabled)}
			if c.KeyStoreAccessOption.KeyAccessRoleArn != nil {
				o["KeyAccessRoleArn"] = *c.KeyStoreAccessOption.KeyAccessRoleArn
			}
			config["KeyStoreAccessOption"] = o
		}
		properties["AssociationConfiguration"] = config
	}
	f.store.Put(packageAssociationType, key, properties)
	end := types.DomainPackageStatusActive
	if f.failAssociate {
		end, f.failAssociate = types.DomainPackageStatusAssociationFailed, false
	}
	f.assocChanges[key] = &pkgAssocChange{status: types.DomainPackageStatusAssociating, end: end}
	d := f.assocDetails(key)
	return &opensearch.AssociatePackageOutput{DomainPackageDetails: &d}, nil
}

func anyStrings(list []string) []any {
	out := []any{}
	for _, v := range list {
		out = append(out, v)
	}
	return out
}

func (f *pkgFake) DissociatePackage(ctx context.Context, in *opensearch.DissociatePackageInput, _ ...func(*opensearch.Options)) (*opensearch.DissociatePackageOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DissociatePackage"); err != nil {
		return nil, err
	}
	domain, id := aws.ToString(in.DomainName), aws.ToString(in.PackageID)
	if !packageIDPattern.MatchString(id) {
		return nil, pkgFault(pkgValidationCode, "PackageID %q doesn't match the pattern", id)
	}
	if err := checkDomain(domain); err != nil {
		return nil, err
	}
	key := domain + "|" + id
	if f.store.Resource(packageAssociationType, key) == nil {
		return nil, pkgFault("ResourceNotFoundException", "the domain %q doesn't have the package %q", domain, id)
	}
	if f.domainBusy(domain) {
		return nil, pkgFault("ConflictException", "the domain %q is processing a change", domain)
	}
	end := types.DomainPackageStatus("")
	if f.failDissociate {
		end, f.failDissociate = types.DomainPackageStatusDissociationFailed, false
	}
	f.assocChanges[key] = &pkgAssocChange{status: types.DomainPackageStatusDissociating, end: end}
	d := f.assocDetails(key)
	return &opensearch.DissociatePackageOutput{DomainPackageDetails: &d}, nil
}

func (f *pkgFake) ListPackagesForDomain(ctx context.Context, in *opensearch.ListPackagesForDomainInput, _ ...func(*opensearch.Options)) (*opensearch.ListPackagesForDomainOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("ListPackagesForDomain"); err != nil {
		return nil, err
	}
	domain := aws.ToString(in.DomainName)
	if err := checkDomain(domain); err != nil {
		return nil, err
	}
	if in.MaxResults > 100 {
		return nil, pkgFault(pkgValidationCode, "MaxResults must be at most 100")
	}
	list := []types.DomainPackageDetails{}
	for _, key := range f.associations(domain) {
		list = append(list, f.assocDetails(key))
	}
	items, next := pkgPage(list, in.NextToken)
	for _, d := range items {
		f.settleAssoc(aws.ToString(d.DomainName) + "|" + aws.ToString(d.PackageID))
	}
	return &opensearch.ListPackagesForDomainOutput{DomainPackageDetailsList: items, NextToken: next}, nil
}

func (f *pkgFake) GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, _ ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetCallerIdentity"); err != nil {
		return nil, err
	}
	return &sts.GetCallerIdentityOutput{Account: aws.String(pkgAccount)}, nil
}

func (f *pkgFake) GetPackageVersionHistory(ctx context.Context, in *opensearch.GetPackageVersionHistoryInput, _ ...func(*opensearch.Options)) (*opensearch.GetPackageVersionHistoryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("GetPackageVersionHistory"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.PackageID)
	if !packageIDPattern.MatchString(id) {
		return nil, pkgFault(pkgValidationCode, "PackageID %q doesn't match the pattern", id)
	}
	if in.MaxResults > 100 {
		return nil, pkgFault(pkgValidationCode, "MaxResults must be at most 100")
	}
	if f.lookup(id) == nil {
		return nil, pkgFault("ResourceNotFoundException", "the package %q doesn't exist", id)
	}
	items, next := pkgPage(f.history[id], in.NextToken)
	return &opensearch.GetPackageVersionHistoryOutput{PackageID: aws.String(id), PackageVersionHistoryList: items, NextToken: next}, nil
}
