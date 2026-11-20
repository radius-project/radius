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
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const outboundConnectionType = "AWS::OpenSearchService::OutboundConnection"

// ConnectionAPI is the part of the Amazon OpenSearch Service client the outbound connection handler uses.
type ConnectionAPI interface {
	CreateOutboundConnection(ctx context.Context, params *opensearch.CreateOutboundConnectionInput, optFns ...func(*opensearch.Options)) (*opensearch.CreateOutboundConnectionOutput, error)
	DescribeOutboundConnections(ctx context.Context, params *opensearch.DescribeOutboundConnectionsInput, optFns ...func(*opensearch.Options)) (*opensearch.DescribeOutboundConnectionsOutput, error)
	DeleteOutboundConnection(ctx context.Context, params *opensearch.DeleteOutboundConnectionInput, optFns ...func(*opensearch.Options)) (*opensearch.DeleteOutboundConnectionOutput, error)
}

var (
	_ native.Handler           = (*OutboundConnection)(nil)
	_ native.IdentifierEncoder = (*OutboundConnection)(nil)
	_ native.Finder            = (*OutboundConnection)(nil)
)

// The documented patterns of the values a connection takes. AWS's patterns match the whole value.
var (
	connectionIDPattern       = regexp.MustCompile(`^[a-z][a-z0-9\-]+$`)
	connectionAliasPattern    = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9\-_]+$`)
	connectionNamePattern     = regexp.MustCompile(`^[a-z][a-z0-9\-]+$`)
	connectionOwnerPattern    = regexp.MustCompile(`^[0-9]{12}$`)
	connectionEndpointPattern = regexp.MustCompile(`^[A-Za-z0-9\-.]+$`)
)

// liveConnectionStatuses lists the statuses of a connection that is, or may become, usable. The others are failed, rejected,
// or deleted, or on their way there.
var liveConnectionStatuses = []types.OutboundConnectionStatusCode{
	types.OutboundConnectionStatusCodeValidating,
	types.OutboundConnectionStatusCodePendingAcceptance,
	types.OutboundConnectionStatusCodeApproved,
	types.OutboundConnectionStatusCodeProvisioning,
	types.OutboundConnectionStatusCodeActive,
}

// OutboundConnection serves AWS::OpenSearchService::OutboundConnection: a cross-cluster connection from a source
// domain in the caller's account and Region to a destination domain. Its identifier, ConnectionId, is the ID AWS
// assigns. AWS has no call that changes a connection, so every property is create-only, and UCP rejects a change with
// 400 before any AWS call. The create takes no idempotency token, so a lost create is found by the source domain, the
// alias, and the destination domain, and AWS rejects a second connection with the same values.
type OutboundConnection struct {
	api ConnectionAPI
}

// NewOutboundConnection returns the handler for OpenSearch Service outbound connections.
func NewOutboundConnection(api ConnectionAPI) *OutboundConnection {
	return &OutboundConnection{api: api}
}

// TypeName returns AWS::OpenSearchService::OutboundConnection.
func (h *OutboundConnection) TypeName() string {
	return outboundConnectionType
}

// Options returns how UCP uses the handler.
func (h *OutboundConnection) Options() native.Options {
	return native.Options{List: native.ListRegion, Recovery: native.RecoverBySearch}
}

// connectionDomain is a domain a template names. owner and region are "" when the template leaves them out.
type connectionDomain struct {
	name, owner, region string
}

// connectionTemplate is the connection a template describes. Values the template leaves out are "".
type connectionTemplate struct {
	alias           string
	local, remote   connectionDomain
	mode            types.ConnectionMode
	skipUnavailable types.SkipUnavailableStatus
	endpoint        string
}

// connectionObject returns value as an object with only the keys names, or a client failure naming what.
func connectionObject(value any, what string, names ...string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalid(outboundConnectionType, "%s must be an object", what)
	}
	for name := range object {
		if !slices.Contains(names, name) {
			return nil, invalid(outboundConnectionType, "%s has no property %s. It takes %s", what, name, strings.Join(names, ", "))
		}
	}
	return object, nil
}

// connectionString returns the string at name in object, or "" when it is absent.
func connectionString(object map[string]any, name string, what string) (string, error) {
	value, present := object[name]
	if !present || value == nil {
		return "", nil
	}
	s, ok := value.(string)
	if !ok {
		return "", invalid(outboundConnectionType, "%s must be a string", what)
	}
	return s, nil
}

// parseConnectionDomain returns the domain a LocalDomainInfo or RemoteDomainInfo value names.
func parseConnectionDomain(value any, what string) (connectionDomain, error) {
	container, err := connectionObject(value, what, "AWSDomainInformation")
	if err != nil {
		return connectionDomain{}, err
	}
	what += ".AWSDomainInformation"
	info, err := connectionObject(container["AWSDomainInformation"], what, "DomainName", "OwnerId", "Region")
	if err != nil {
		return connectionDomain{}, err
	}
	strs := map[string]string{}
	for _, name := range []string{"DomainName", "OwnerId", "Region"} {
		if strs[name], err = connectionString(info, name, what+"."+name); err != nil {
			return connectionDomain{}, err
		}
	}
	d := connectionDomain{name: strs["DomainName"], owner: strs["OwnerId"], region: strs["Region"]}
	switch {
	case len(d.name) < 3 || len(d.name) > 28 || !connectionNamePattern.MatchString(d.name):
		return connectionDomain{}, invalid(outboundConnectionType, "%s.DomainName must be 3 to 28 lowercase letters, digits, and hyphens, starting with a letter", what)
	case d.owner != "" && !connectionOwnerPattern.MatchString(d.owner):
		return connectionDomain{}, invalid(outboundConnectionType, "%s.OwnerId must be a 12-digit AWS account ID", what)
	case d.region != "" && (len(d.region) < 5 || len(d.region) > 30 || !connectionNamePattern.MatchString(d.region)):
		return connectionDomain{}, invalid(outboundConnectionType, "%s.Region must be an AWS Region, such as us-west-2", what)
	}
	return d, nil
}

// parseConnection checks every property of the template that it sets and returns the connection. It doesn't
// require any property, because UCP also calls Identifier with only ConnectionId, for `existing`.
func parseConnection(properties map[string]any) (connectionTemplate, error) {
	var c connectionTemplate
	var err error
	if c.alias, err = connectionString(properties, "ConnectionAlias", "ConnectionAlias"); err != nil {
		return c, err
	}
	if c.alias != "" && (len(c.alias) < 2 || len(c.alias) > 100 || !connectionAliasPattern.MatchString(c.alias)) {
		return c, invalid(outboundConnectionType, "ConnectionAlias must be 2 to 100 letters, digits, hyphens, and underscores, starting with a letter")
	}
	if value := properties["LocalDomainInfo"]; value != nil {
		if c.local, err = parseConnectionDomain(value, "LocalDomainInfo"); err != nil {
			return c, err
		}
	}
	if value := properties["RemoteDomainInfo"]; value != nil {
		if c.remote, err = parseConnectionDomain(value, "RemoteDomainInfo"); err != nil {
			return c, err
		}
	}
	mode, err := connectionString(properties, "ConnectionMode", "ConnectionMode")
	if err != nil {
		return c, err
	}
	if c.mode = types.ConnectionMode(mode); mode != "" && !slices.Contains(c.mode.Values(), c.mode) {
		return c, invalid(outboundConnectionType, "ConnectionMode must be one of %v", c.mode.Values())
	}
	if value := properties["ConnectionProperties"]; value != nil {
		props, err := connectionObject(value, "ConnectionProperties", "CrossClusterSearch", "Endpoint")
		if err != nil {
			return c, err
		}
		if c.endpoint, err = connectionString(props, "Endpoint", "ConnectionProperties.Endpoint"); err != nil {
			return c, err
		}
		if c.endpoint != "" && !connectionEndpointPattern.MatchString(c.endpoint) {
			return c, invalid(outboundConnectionType, "ConnectionProperties.Endpoint must be a domain endpoint: letters, digits, hyphens, and dots")
		}
		if value := props["CrossClusterSearch"]; value != nil {
			search, err := connectionObject(value, "ConnectionProperties.CrossClusterSearch", "SkipUnavailable")
			if err != nil {
				return c, err
			}
			skip, err := connectionString(search, "SkipUnavailable", "ConnectionProperties.CrossClusterSearch.SkipUnavailable")
			if err != nil {
				return c, err
			}
			if c.skipUnavailable = types.SkipUnavailableStatus(skip); skip != "" && !slices.Contains(c.skipUnavailable.Values(), c.skipUnavailable) {
				return c, invalid(outboundConnectionType, "ConnectionProperties.CrossClusterSearch.SkipUnavailable must be one of %v", c.skipUnavailable.Values())
			}
		}
	}
	return c, nil
}

// parseConnectionRequest returns the connection a create or a search asks for, with every required property, whose
// source domain is in region.
func parseConnectionRequest(properties map[string]any, region string) (connectionTemplate, error) {
	c, err := parseConnection(properties)
	if err != nil {
		return c, err
	}
	if c.alias == "" || c.local.name == "" || c.remote.name == "" {
		return c, invalid(outboundConnectionType, "set ConnectionAlias, LocalDomainInfo, and RemoteDomainInfo")
	}
	if c.local.region != "" && c.local.region != region {
		return c, invalid(outboundConnectionType, "LocalDomainInfo.AWSDomainInformation.Region is %s, but the source domain must be in the Region of the request, %s", c.local.region, region)
	}
	return c, nil
}

// checkConnectionID returns a client failure for an identifier that isn't a connection ID, so it is never sent.
func checkConnectionID(identifier string) error {
	if len(identifier) < 10 || len(identifier) > 256 || !connectionIDPattern.MatchString(identifier) {
		return invalid(outboundConnectionType, "%q is not the ID of an OpenSearch Service outbound connection", identifier)
	}
	return nil
}

// Identifier returns ConnectionId, or "" before AWS assigns it. UCP calls it before any AWS call, so a template AWS
// would reject is rejected here.
func (h *OutboundConnection) Identifier(properties map[string]any) (string, error) {
	if _, err := parseConnection(properties); err != nil {
		return "", err
	}
	id, err := connectionString(properties, "ConnectionId", "ConnectionId")
	if err != nil || id == "" {
		return "", err
	}
	return id, checkConnectionID(id)
}

// connections returns every outbound connection of the caller's domains in the region. DescribeOutboundConnections
// takes filters, but AWS's API reference doesn't name them, so the handler reads every page and matches itself.
func (h *OutboundConnection) connections(ctx context.Context, region string) ([]types.OutboundConnection, error) {
	out := []types.OutboundConnection{}
	input := &opensearch.DescribeOutboundConnectionsInput{}
	for {
		page, err := h.api.DescribeOutboundConnections(ctx, input, inRegion(region))
		if err != nil {
			return nil, err
		}
		out = append(out, page.Connections...)
		if input.NextToken = page.NextToken; aws.ToString(input.NextToken) == "" {
			return out, nil
		}
	}
}

// connection returns the connection with the identifier, in any status, or ErrNotFound when AWS doesn't list it.
func (h *OutboundConnection) connection(ctx context.Context, region string, identifier string) (*types.OutboundConnection, error) {
	if err := checkConnectionID(identifier); err != nil {
		return nil, err
	}
	all, err := h.connections(ctx, region)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if aws.ToString(all[i].ConnectionId) == identifier {
			return &all[i], nil
		}
	}
	return nil, native.ErrNotFound
}

// connectionStatusOf returns the connection's status code.
func connectionStatusOf(c *types.OutboundConnection) types.OutboundConnectionStatusCode {
	if c.ConnectionStatus == nil {
		return ""
	}
	return c.ConnectionStatus.StatusCode
}

// connectionProperties returns the connection's properties.
func connectionProperties(c *types.OutboundConnection) (map[string]any, error) {
	return awsapi.EncodeObject(c)
}

// Read returns the connection's properties, or ErrNotFound when AWS doesn't list it or reports it DELETED. AWS lists
// a deleted connection for 15 days.
func (h *OutboundConnection) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	c, err := h.connection(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	if connectionStatusOf(c) == types.OutboundConnectionStatusCodeDeleted {
		return nil, native.ErrNotFound
	}
	return connectionProperties(c)
}

// connectionDomainOf returns the domain of a LocalDomainInfo or RemoteDomainInfo AWS returns, with "" for values it leaves out.
func connectionDomainOf(c *types.DomainInformationContainer) connectionDomain {
	if c == nil || c.AWSDomainInformation == nil {
		return connectionDomain{}
	}
	i := c.AWSDomainInformation
	return connectionDomain{name: aws.ToString(i.DomainName), owner: aws.ToString(i.OwnerId), region: aws.ToString(i.Region)}
}

// sameConnectionDomain reports whether AWS's domain got is the template's want. A value the template leaves out is the default
// def: the source domain's owner and Region, where the source domain is in the caller's account and Region.
func sameConnectionDomain(want connectionDomain, got connectionDomain, def connectionDomain) bool {
	owner, region := want.owner, want.region
	if owner == "" {
		owner = def.owner
	}
	if region == "" {
		region = def.region
	}
	return got.name == want.name && got.owner == owner && got.region == region
}

// matches reports whether AWS's connection c is one the template's create would make in region: the same source
// domain, alias, and destination domain. The other values are compared by sameSettings.
func (t connectionTemplate) matches(c *types.OutboundConnection, region string) bool {
	local := connectionDomainOf(c.LocalDomainInfo)
	// Every connection AWS lists is from a domain of the caller, so the source domain's owner is the caller.
	def := connectionDomain{owner: local.owner, region: region}
	return aws.ToString(c.ConnectionAlias) == t.alias && sameConnectionDomain(t.local, local, def) && sameConnectionDomain(t.remote, connectionDomainOf(c.RemoteDomainInfo), def)
}

// sameSettings returns a rejected client failure when the connection has another value than one the template sets,
// so a create never takes another connection as the template's.
func (t connectionTemplate) sameSettings(c *types.OutboundConnection) error {
	different := []string{}
	if t.mode != "" && c.ConnectionMode != t.mode {
		different = append(different, "ConnectionMode")
	}
	var props types.ConnectionProperties
	if c.ConnectionProperties != nil {
		props = *c.ConnectionProperties
	}
	if t.endpoint != "" && aws.ToString(props.Endpoint) != t.endpoint {
		different = append(different, "ConnectionProperties.Endpoint")
	}
	if t.skipUnavailable != "" && (props.CrossClusterSearch == nil || props.CrossClusterSearch.SkipUnavailable != t.skipUnavailable) {
		different = append(different, "ConnectionProperties.CrossClusterSearch.SkipUnavailable")
	}
	if len(different) == 0 {
		return nil
	}
	return &native.Failure{
		Message: fmt.Sprintf("%s: the source domain %q already has the connection %q (%s) named %q to the domain %q, with another %s than the template's. "+
			"Set the template's values to match, or delete that connection first",
			outboundConnectionType, t.local.name, aws.ToString(c.ConnectionId), connectionStatusOf(c), t.alias, t.remote.name, strings.Join(different, ", ")),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// failedConnectionStatuses lists the statuses of a connection that failed, which AWS keeps listing.
var failedConnectionStatuses = []types.OutboundConnectionStatusCode{
	types.OutboundConnectionStatusCodeValidationFailed,
	types.OutboundConnectionStatusCodeRejecting,
	types.OutboundConnectionStatusCodeRejected,
}

// connectionSearch is what a search of the source domain's connections found.
type connectionSearch struct {
	// live and failed are the connections that match the template, by status. others are the connections of the
	// source domain that hold the alias in any other way, such as to another destination domain, or while AWS is
	// deleting them.
	live, failed, others []types.OutboundConnection
}

// search returns the source domain's connections that hold the template's alias, except those AWS deleted.
func (h *OutboundConnection) search(ctx context.Context, region string, t connectionTemplate) (connectionSearch, error) {
	all, err := h.connections(ctx, region)
	if err != nil {
		return connectionSearch{}, err
	}
	var s connectionSearch
	for _, c := range all {
		status := connectionStatusOf(&c)
		switch {
		case status == types.OutboundConnectionStatusCodeDeleted:
		case slices.Contains(liveConnectionStatuses, status) && t.matches(&c, region):
			s.live = append(s.live, c)
		case slices.Contains(failedConnectionStatuses, status) && t.matches(&c, region):
			s.failed = append(s.failed, c)
		case aws.ToString(c.ConnectionAlias) == t.alias && connectionDomainOf(c.LocalDomainInfo).name == t.local.name:
			s.others = append(s.others, c)
		}
	}
	return s, nil
}

// Find returns the ID of the connection from the template's source domain with its alias to its destination domain,
// live or failed, or "" when AWS lists none. AWS keeps a connection that failed, so a delete after a lost create
// removes it. It fails when more than one matches, because any of them could be the lost create's, or the one that
// matches has another value the template sets. A connection AWS made moments ago may not be listed yet, which the create's duplicate
// rejection covers.
func (h *OutboundConnection) Find(ctx context.Context, request native.CreateRequest) (string, error) {
	t, err := parseConnectionRequest(request.Properties, request.Region)
	if err != nil {
		return "", err
	}
	s, err := h.search(ctx, request.Region, t)
	if err != nil {
		return "", err
	}
	return t.one(slices.Concat(s.live, s.failed))
}

// one returns the ID of the only connection in found, or "" when found is empty. It returns no ID with an error,
// because UCP keeps an identifier Find returns with one.
func (t connectionTemplate) one(found []types.OutboundConnection) (string, error) {
	switch len(found) {
	case 0:
		return "", nil
	case 1:
		if err := t.sameSettings(&found[0]); err != nil {
			return "", err
		}
		return aws.ToString(found[0].ConnectionId), nil
	}
	ids := []string{}
	for _, c := range found {
		ids = append(ids, aws.ToString(c.ConnectionId))
	}
	return "", awsapi.Ambiguous("OpenSearch Service outbound connections", ids)
}

// connectionBusy returns the conflict of a change AWS can't make now because it is changing the connection or its
// source domain. UCP answers 409 and sends nothing more, so the caller tries again later.
func connectionBusy(what string, cause error) error {
	message := fmt.Sprintf("AWS is changing %s, so nothing was changed. Try again once it settles, and give the connection dependsOn on its domains", what)
	if cause != nil {
		message += ": " + cause.Error()
	}
	return &native.Failure{Message: message, ClientError: true, Conflict: true, Outcome: native.OutcomeRejected}
}

// Create creates the connection. AWS rejects a second connection from the source domain with the same alias, such as
// one a lost create made, with ResourceAlreadyExistsException. Create then reports what AWS has, and never returns
// that connection, because UCP takes a created identifier as the record's without checking that no other alias
// manages it. After a lost create, the next deploy finds it with Find, which UCP checks.
func (h *OutboundConnection) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	t, err := parseConnectionRequest(request.Properties, request.Region)
	if err != nil {
		return "", err
	}
	input := &opensearch.CreateOutboundConnectionInput{
		ConnectionAlias:  aws.String(t.alias),
		LocalDomainInfo:  t.local.sdk(),
		RemoteDomainInfo: t.remote.sdk(),
		ConnectionMode:   t.mode,
	}
	if t.endpoint != "" || t.skipUnavailable != "" {
		input.ConnectionProperties = &types.ConnectionProperties{}
		if t.endpoint != "" {
			input.ConnectionProperties.Endpoint = aws.String(t.endpoint)
		}
		if t.skipUnavailable != "" {
			input.ConnectionProperties.CrossClusterSearch = &types.CrossClusterSearchConnectionProperties{SkipUnavailable: t.skipUnavailable}
		}
	}
	// The call takes no token, so the SDK must not send it again on its own: after a response lost on the way back,
	// a retry would get ResourceAlreadyExistsException, which reads as a create AWS rejected.
	out, err := h.api.CreateOutboundConnection(ctx, input, inRegion(request.Region), func(o *opensearch.Options) { o.RetryMaxAttempts = 1 })
	switch {
	case awsapi.HasCode(err, "ResourceAlreadyExistsException"):
		return h.existing(ctx, request.Region, t, err)
	case awsapi.HasCode(err, "ConflictException"):
		return "", connectionBusy(fmt.Sprintf("the domain %q", t.local.name), err)
	case err != nil:
		return "", awsapi.Rejected(err)
	}
	if aws.ToString(out.ConnectionId) == "" {
		return "", fmt.Errorf("AWS created an OpenSearch Service outbound connection from %q but returned no ID", t.local.name)
	}
	return aws.ToString(out.ConnectionId), nil
}

// existing returns the error of a create AWS rejected with cause, because the source domain already has a connection
// with the alias. AWS changed nothing, so every error is rejected. A connection that matches the template is a
// conflict: an earlier attempt of this create may have made it, and the next deploy finds it.
func (h *OutboundConnection) existing(ctx context.Context, region string, t connectionTemplate, cause error) (string, error) {
	s, err := h.search(ctx, region, t)
	if err != nil {
		return "", beforeChange(err)
	}
	if len(s.live) > 0 {
		id, err := t.one(slices.Concat(s.live, s.failed))
		if err != nil {
			return "", beforeChange(err)
		}
		return "", &native.Failure{
			Message: fmt.Sprintf("%s: AWS already has the connection %q named %q from the domain %q to the domain %q. If an earlier deploy of this resource made it, "+
				"deploy again to take it. Otherwise another resource manages it, or it was made outside Radius: use another ConnectionAlias, or delete that connection first",
				outboundConnectionType, id, t.alias, t.local.name, t.remote.name),
			ClientError: true,
			Conflict:    true,
			Outcome:     native.OutcomeRejected,
		}
	}
	if held := slices.Concat(s.failed, s.others); len(held) > 0 {
		names := []string{}
		for _, c := range held {
			names = append(names, fmt.Sprintf("%s (%s, to %q)", aws.ToString(c.ConnectionId), connectionStatusOf(&c), connectionDomainOf(c.RemoteDomainInfo).name))
		}
		return "", &native.Failure{
			Message: fmt.Sprintf("%s: the source domain %q already has a connection named %q that AWS doesn't let this create reuse: %s. Use another ConnectionAlias, or delete that connection first: %s",
				outboundConnectionType, t.local.name, t.alias, strings.Join(names, ", "), cause.Error()),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	// AWS may not list a connection it made moments ago, such as one a lost create made.
	return "", connectionBusy(fmt.Sprintf("the connection named %q from the domain %q, which AWS reports exists but doesn't list yet", t.alias, t.local.name), cause)
}

// sdk returns the domain as the AWS SDK takes it.
func (d connectionDomain) sdk() *types.DomainInformationContainer {
	i := &types.AWSDomainInformation{DomainName: aws.String(d.name)}
	if d.owner != "" {
		i.OwnerId = aws.String(d.owner)
	}
	if d.region != "" {
		i.Region = aws.String(d.region)
	}
	return &types.DomainInformationContainer{AWSDomainInformation: i}
}

// Update changes nothing, because every property is create-only.
func (h *OutboundConnection) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// connectionSettling reports whether AWS is still moving the connection to another status on its own.
func connectionSettling(status types.OutboundConnectionStatusCode) bool {
	switch status {
	case types.OutboundConnectionStatusCodeValidating, types.OutboundConnectionStatusCodeApproved,
		types.OutboundConnectionStatusCodeProvisioning, types.OutboundConnectionStatusCodeRejecting:
		return true
	}
	return false
}

// Delete starts deleting the connection. A connection AWS is already deleting had a delete reach AWS before, so the
// delete succeeds. A connection AWS is still validating, provisioning, or rejecting gets a conflict, and nothing is
// sent.
func (h *OutboundConnection) Delete(ctx context.Context, region string, identifier string) error {
	c, err := h.connection(ctx, region, identifier)
	if err != nil {
		return beforeChange(err)
	}
	switch status := connectionStatusOf(c); {
	case status == types.OutboundConnectionStatusCodeDeleted:
		return native.ErrNotFound
	case status == types.OutboundConnectionStatusCodeDeleting:
		return nil
	case connectionSettling(status):
		return connectionBusy(fmt.Sprintf("the outbound connection %q (%s)", identifier, status), nil)
	}
	_, err = h.api.DeleteOutboundConnection(ctx, &opensearch.DeleteOutboundConnectionInput{ConnectionId: aws.String(identifier)}, inRegion(region))
	if awsapi.HasCode(err, "ConflictException") {
		return connectionBusy(fmt.Sprintf("the outbound connection %q or its domain", identifier), err)
	}
	return awsapi.Rejected(awsapi.NotFound(err, "ResourceNotFoundException"))
}

// List returns every connection in the region except those AWS is deleting or deleted.
func (h *OutboundConnection) List(ctx context.Context, region string) ([]native.Resource, error) {
	all, err := h.connections(ctx, region)
	if err != nil {
		return nil, err
	}
	out := []native.Resource{}
	for i := range all {
		if status := connectionStatusOf(&all[i]); status == types.OutboundConnectionStatusCodeDeleting || status == types.OutboundConnectionStatusCodeDeleted {
			continue
		}
		properties, err := connectionProperties(&all[i])
		if err != nil {
			return nil, err
		}
		out = append(out, native.Resource{Identifier: aws.ToString(all[i].ConnectionId), Properties: properties})
	}
	return out, nil
}

// Progress reads the connection's status.
//
// A create is done once AWS validated the connection: PENDING_ACCEPTANCE, or a later status of an accepted connection.
// A connection to a domain of another owner waits for that owner to accept it, which may never happen, so Radius
// doesn't wait for ACTIVE. The create fails at VALIDATION_FAILED, or when the destination domain's owner rejects it,
// and the connection stays in AWS, so the record stays and a later delete removes it. AWS lists a deleted connection
// as DELETED, so a create that AWS doesn't list yet waits.
//
// A delete waits for DELETED, or for the connection to be gone. An update changes nothing.
func (h *OutboundConnection) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op == native.OperationUpdate {
		return native.Progress{Done: true}, nil
	}
	c, err := h.connection(ctx, region, identifier)
	notListed := op == native.OperationCreate && errors.Is(err, native.ErrNotFound)
	switch {
	case notListed:
		return native.Progress{State: "not listed", Waiting: string(types.OutboundConnectionStatusCodePendingAcceptance)}, nil
	case err != nil:
		return native.Progress{}, err
	}
	status := connectionStatusOf(c)
	if status == types.OutboundConnectionStatusCodeDeleted {
		return native.Progress{}, native.ErrNotFound
	}
	if op == native.OperationDelete {
		return native.Progress{State: string(status), Waiting: string(types.OutboundConnectionStatusCodeDeleted)}, nil
	}
	switch status {
	case types.OutboundConnectionStatusCodePendingAcceptance, types.OutboundConnectionStatusCodeApproved,
		types.OutboundConnectionStatusCodeProvisioning, types.OutboundConnectionStatusCodeActive:
		return native.Progress{Done: true, State: string(status)}, nil
	case types.OutboundConnectionStatusCodeValidationFailed, types.OutboundConnectionStatusCodeRejecting,
		types.OutboundConnectionStatusCodeRejected, types.OutboundConnectionStatusCodeDeleting:
		cause := "Check that both domains meet the requirements of cross-cluster search"
		switch status {
		case types.OutboundConnectionStatusCodeRejecting, types.OutboundConnectionStatusCodeRejected:
			cause = "The destination domain's owner rejected it"
		case types.OutboundConnectionStatusCodeDeleting:
			cause = "Someone deleted it"
		}
		if c.ConnectionStatus != nil && aws.ToString(c.ConnectionStatus.Message) != "" {
			cause = aws.ToString(c.ConnectionStatus.Message) + ". " + cause
		}
		return native.Progress{Done: true, State: string(status), Failure: &native.Failure{
			Message: fmt.Sprintf("The OpenSearch Service outbound connection %q is %s. %s, then delete the resource and deploy again", identifier, status, cause),
		}}, nil
	}
	return native.Progress{State: string(status), Waiting: string(types.OutboundConnectionStatusCodePendingAcceptance)}, nil
}
