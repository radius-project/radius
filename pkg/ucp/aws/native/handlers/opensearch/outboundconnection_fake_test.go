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
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const (
	// connAccount is the fake's caller, and connOtherAccount another account.
	connAccount      = "123456789012"
	connOtherAccount = "210987654321"

	connRegion = "us-west-2"

	// connPageSize is how many connections a page has. AWS doesn't document its page size.
	connPageSize = 2

	// connListLag is how many DescribeOutboundConnections calls leave out a new connection. AWS doesn't say whether
	// it lists a connection as soon as it makes it.
	connListLag = 1

	// connSettleReads is how many reads a connection stays in a status AWS moves on from on its own, such as
	// VALIDATING, before it moves on. The first read never sees it settled.
	connSettleReads = 2

	// connMaxOutgoing is the documented limit of outgoing connections of a domain.
	connMaxOutgoing = 20

	// connValidationCode is the error the Common Errors page gives for input that fails a constraint, which the fake
	// returns where a call's page names no error for it.
	connValidationCode = "ValidationError"

	// connBusyCode is the error the fake returns for a call while the domain or the connection is changing. The
	// calls' pages name no error for it.
	connBusyCode = "ConflictException"
)

// connEngine is a domain's engine and version, such as OpenSearch 2.11 or Elasticsearch 7.10.
type connEngine struct {
	opensearch   bool
	major, minor int
}

// atLeast reports whether e is the same engine as o, at the same or a higher version.
func (e connEngine) atLeast(o connEngine) bool {
	return e.opensearch == o.opensearch && (e.major > o.major || (e.major == o.major && e.minor >= o.minor))
}

// connDomainSpec is a domain the fake knows, with what cross-cluster search requires of it.
type connDomainSpec struct {
	engine      connEngine
	fineGrained bool
	nodeToNode  bool
}

// connDomainKey names a domain: its owner, Region, and name.
func connDomainKey(owner, region, name string) string {
	return owner + "/" + region + "/" + name
}

var (
	os211 = connEngine{opensearch: true, major: 2, minor: 11}
	os113 = connEngine{opensearch: true, major: 1, minor: 3}
	es79  = connEngine{major: 7, minor: 9}

	// connDomains are the domains the fake knows, by connDomainKey.
	connDomains = map[string]connDomainSpec{
		connDomainKey(connAccount, connRegion, "logs"):         {engine: os211, fineGrained: true, nodeToNode: true},
		connDomainKey(connAccount, connRegion, "archive"):      {engine: os211, fineGrained: true, nodeToNode: true},
		connDomainKey(connAccount, connRegion, "metrics"):      {engine: os211, fineGrained: true, nodeToNode: true},
		connDomainKey(connAccount, connRegion, "old"):          {engine: os113, fineGrained: true, nodeToNode: true},
		connDomainKey(connAccount, connRegion, "plain"):        {engine: os211, fineGrained: false, nodeToNode: true},
		connDomainKey(connAccount, connRegion, "legacy"):       {engine: es79, fineGrained: true, nodeToNode: true},
		connDomainKey(connAccount, "us-east-1", "far"):         {engine: os211, fineGrained: true, nodeToNode: true},
		connDomainKey(connAccount, "us-east-1", "far-legacy"):  {engine: es79, fineGrained: true, nodeToNode: true},
		connDomainKey(connOtherAccount, connRegion, "shared"):  {engine: os211, fineGrained: true, nodeToNode: true},
		connDomainKey(connOtherAccount, connRegion, "shared2"): {engine: os211, fineGrained: true, nodeToNode: true},
	}
)

// connFake is an in-memory Amazon OpenSearch Service API for outbound connections. It keeps connections in an
// awsfake store, in their type's property format, so the suite's Cloud Control sees the same resources, and tolerates
// a record the suite writes there with only some values. It follows the rules in connAPI, and where AWS's docs are
// silent it takes the pessimistic side:
//
//   - A new connection is left out of lagReads reads, then stays VALIDATING for connSettleReads reads. A valid
//     one is then PENDING_ACCEPTANCE until the destination domain's owner accepts or rejects it, even for a domain of
//     the caller. An accepted one is APPROVED and PROVISIONING before it is ACTIVE.
//   - A create is a duplicate only when a live connection has the same source domain, alias, and destination domain.
//   - While a domain is processing a change of its own, or a connection is moving on its own, a create from the
//     domain or a delete of the connection is rejected with connBusyCode.
//   - A deleted connection is DELETING for connSettleReads reads, then listed as DELETED from then on.
type connFake struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// status holds each connection's status. A record with none, such as one the suite wrote, is ACTIVE.
	status map[string]types.OutboundConnectionStatusCode
	reads  map[string]int

	// read counts the reads of the list, and created holds the read count when each connection was made. A new
	// connection is left out of lagReads reads.
	read     int
	created  map[string]int
	lagReads int

	// deleted holds the connections AWS deleted, which it still lists as DELETED.
	deleted map[string]map[string]any

	nextID int

	// processing is how many more calls each domain, by connDomainKey, stays processing a change of its own.
	processing map[string]int

	// hold keeps connections in the status they are in.
	hold bool

	// calls counts the calls that reached the fake, by method, and errs maps a method to an error it returns
	// instead of acting.
	calls map[string]int
	errs  map[string]error
}

var _ ConnectionAPI = (*connFake)(nil)

func newConnFake(store *awsfake.CloudControl) *connFake {
	return &connFake{
		store:      store,
		status:     map[string]types.OutboundConnectionStatusCode{},
		reads:      map[string]int{},
		created:    map[string]int{},
		lagReads:   connListLag,
		deleted:    map[string]map[string]any{},
		processing: map[string]int{},
		calls:      map[string]int{},
		errs:       map[string]error{},
	}
}

func connFault(code string, format string, args ...any) error {
	return &smithy.GenericAPIError{Code: code, Message: fmt.Sprintf(format, args...), Fault: smithy.FaultClient}
}

func (f *connFake) begin(method string) error {
	f.calls[method]++
	return f.errs[method]
}

// tick passes time for the domains processing a change of their own.
func (f *connFake) tick() {
	for name, n := range f.processing {
		if n <= 1 {
			delete(f.processing, name)
		} else {
			f.processing[name] = n - 1
		}
	}
}

// statusOf returns the connection's status.
func (f *connFake) statusOf(id string) types.OutboundConnectionStatusCode {
	if _, gone := f.deleted[id]; gone {
		return types.OutboundConnectionStatusCodeDeleted
	}
	if status, ok := f.status[id]; ok {
		return status
	}
	return types.OutboundConnectionStatusCodeActive
}

// connDomainValues returns the owner, Region, and name of the domain at name in a stored connection, with the
// defaults for values it leaves out.
func connDomainValues(p map[string]any, name string) (owner, region, domain string) {
	container, _ := p[name].(map[string]any)
	info, _ := container["AWSDomainInformation"].(map[string]any)
	owner, _ = info["OwnerId"].(string)
	region, _ = info["Region"].(string)
	domain, _ = info["DomainName"].(string)
	if owner == "" {
		owner = connAccount
	}
	if region == "" {
		region = connRegion
	}
	return owner, region, domain
}

// connKey returns the source domain, alias, and destination domain of a stored connection.
func connKey(p map[string]any) string {
	lo, lr, ln := connDomainValues(p, "LocalDomainInfo")
	ro, rr, rn := connDomainValues(p, "RemoteDomainInfo")
	alias, _ := p["ConnectionAlias"].(string)
	return connDomainKey(lo, lr, ln) + "|" + alias + "|" + connDomainKey(ro, rr, rn)
}

// checkInfo checks a domain's information against AWSDomainInformation's documented constraints.
func checkInfo(c *types.DomainInformationContainer, what string) error {
	if c == nil || c.AWSDomainInformation == nil {
		return connFault(connValidationCode, "%s is required", what)
	}
	i := c.AWSDomainInformation
	name := aws.ToString(i.DomainName)
	if len(name) < 3 || len(name) > 28 || !connectionNamePattern.MatchString(name) {
		return connFault(connValidationCode, "%s.DomainName %q doesn't match the pattern", what, name)
	}
	if i.OwnerId != nil && !connectionOwnerPattern.MatchString(*i.OwnerId) {
		return connFault(connValidationCode, "%s.OwnerId %q isn't 12 digits", what, *i.OwnerId)
	}
	if r := aws.ToString(i.Region); i.Region != nil && (len(r) < 5 || len(r) > 30 || !connectionNamePattern.MatchString(r)) {
		return connFault(connValidationCode, "%s.Region %q doesn't match the pattern", what, r)
	}
	return nil
}

// storedConnection returns the connection a create stores, with AWS's defaults filled in.
func storedConnection(id string, in *opensearch.CreateOutboundConnectionInput) map[string]any {
	domain := func(c *types.DomainInformationContainer) map[string]any {
		i := c.AWSDomainInformation
		owner, region := aws.ToString(i.OwnerId), aws.ToString(i.Region)
		if owner == "" {
			owner = connAccount
		}
		if region == "" {
			region = connRegion
		}
		return map[string]any{"AWSDomainInformation": map[string]any{"DomainName": aws.ToString(i.DomainName), "OwnerId": owner, "Region": region}}
	}
	mode := in.ConnectionMode
	if mode == "" {
		mode = types.ConnectionModeDirect
	}
	skip := types.SkipUnavailableStatusDisabled
	properties := map[string]any{}
	if p := in.ConnectionProperties; p != nil {
		if p.CrossClusterSearch != nil && p.CrossClusterSearch.SkipUnavailable != "" {
			skip = p.CrossClusterSearch.SkipUnavailable
		}
		if p.Endpoint != nil {
			properties["Endpoint"] = aws.ToString(p.Endpoint)
		}
	}
	properties["CrossClusterSearch"] = map[string]any{"SkipUnavailable": string(skip)}
	return map[string]any{
		"ConnectionId":         id,
		"ConnectionAlias":      aws.ToString(in.ConnectionAlias),
		"ConnectionMode":       string(mode),
		"ConnectionProperties": properties,
		"LocalDomainInfo":      domain(in.LocalDomainInfo),
		"RemoteDomainInfo":     domain(in.RemoteDomainInfo),
	}
}

// connValid reports whether the domains of a stored connection meet cross-cluster search's requirements.
func connValid(p map[string]any) bool {
	lo, lr, ln := connDomainValues(p, "LocalDomainInfo")
	ro, rr, rn := connDomainValues(p, "RemoteDomainInfo")
	local, ok := connDomains[connDomainKey(lo, lr, ln)]
	remote, remoteOK := connDomains[connDomainKey(ro, rr, rn)]
	switch {
	case !ok || !remoteOK || lo != connAccount || lr != connRegion:
		return false
	case !local.fineGrained || !local.nodeToNode || !remote.fineGrained || !remote.nodeToNode:
		return false
	case !local.engine.atLeast(remote.engine):
		return false
	case lr != rr && (!local.engine.opensearch && !local.engine.atLeast(connEngine{major: 7, minor: 10})):
		return false
	}
	return true
}

func (f *connFake) CreateOutboundConnection(ctx context.Context, in *opensearch.CreateOutboundConnectionInput, _ ...func(*opensearch.Options)) (*opensearch.CreateOutboundConnectionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("CreateOutboundConnection"); err != nil {
		return nil, err
	}
	alias := aws.ToString(in.ConnectionAlias)
	if len(alias) < 2 || len(alias) > 100 || !connectionAliasPattern.MatchString(alias) {
		return nil, connFault(connValidationCode, "ConnectionAlias %q doesn't match the pattern", alias)
	}
	if err := checkInfo(in.LocalDomainInfo, "LocalDomainInfo"); err != nil {
		return nil, err
	}
	if err := checkInfo(in.RemoteDomainInfo, "RemoteDomainInfo"); err != nil {
		return nil, err
	}
	if in.ConnectionMode != "" && !slices.Contains(in.ConnectionMode.Values(), in.ConnectionMode) {
		return nil, connFault(connValidationCode, "ConnectionMode %q isn't a valid value", in.ConnectionMode)
	}
	if p := in.ConnectionProperties; p != nil {
		if p.Endpoint != nil && !connectionEndpointPattern.MatchString(*p.Endpoint) {
			return nil, connFault(connValidationCode, "Endpoint %q doesn't match the pattern", *p.Endpoint)
		}
		if s := p.CrossClusterSearch; s != nil && s.SkipUnavailable != "" && !slices.Contains(s.SkipUnavailable.Values(), s.SkipUnavailable) {
			return nil, connFault(connValidationCode, "SkipUnavailable %q isn't a valid value", s.SkipUnavailable)
		}
	}
	f.nextID++
	id := fmt.Sprintf("cc-%012d", f.nextID)
	properties := storedConnection(id, in)
	key := connKey(properties)
	lo, lr, ln := connDomainValues(properties, "LocalDomainInfo")
	if f.processing[connDomainKey(lo, lr, ln)] > 0 {
		return nil, connFault(connBusyCode, "the domain %q is processing a change", ln)
	}
	outgoing := 0
	for _, other := range f.store.List(outboundConnectionType) {
		p := f.store.Resource(outboundConnectionType, other)
		if slices.Contains(liveConnectionStatuses, f.statusOf(other)) && connKey(p) == key {
			return nil, connFault("ResourceAlreadyExistsException", "the connection %q already exists", other)
		}
		if olo, olr, oln := connDomainValues(p, "LocalDomainInfo"); connDomainKey(olo, olr, oln) == connDomainKey(lo, lr, ln) {
			outgoing++
		}
	}
	if outgoing >= connMaxOutgoing {
		return nil, connFault("LimitExceededException", "the domain %q has %d outgoing connections", ln, outgoing)
	}
	f.store.Put(outboundConnectionType, id, properties)
	f.status[id] = types.OutboundConnectionStatusCodeValidating
	f.created[id] = f.read
	c := f.connection(id)
	return &opensearch.CreateOutboundConnectionOutput{
		ConnectionId:         c.ConnectionId,
		ConnectionAlias:      c.ConnectionAlias,
		ConnectionMode:       c.ConnectionMode,
		ConnectionProperties: c.ConnectionProperties,
		ConnectionStatus:     c.ConnectionStatus,
		LocalDomainInfo:      c.LocalDomainInfo,
		RemoteDomainInfo:     c.RemoteDomainInfo,
	}, nil
}

// connection returns the connection as AWS returns it.
func (f *connFake) connection(id string) *types.OutboundConnection {
	p := f.store.Resource(outboundConnectionType, id)
	if p == nil {
		p = f.deleted[id]
	}
	c := &types.OutboundConnection{}
	if err := awsapi.Decode(p, c); err != nil {
		panic(err)
	}
	c.ConnectionId = aws.String(id)
	c.ConnectionStatus = &types.OutboundConnectionStatus{StatusCode: f.statusOf(id)}
	if c.ConnectionStatus.StatusCode == types.OutboundConnectionStatusCodeValidationFailed {
		c.ConnectionStatus.Message = aws.String("Domains don't meet the requirements of cross-cluster search")
	}
	return c
}

// settle moves the status of a connection a read returned, once it was read connSettleReads times in a status AWS
// moves on from on its own.
func (f *connFake) settle(id string) {
	status := f.statusOf(id)
	next := map[types.OutboundConnectionStatusCode]types.OutboundConnectionStatusCode{
		types.OutboundConnectionStatusCodeValidating:   types.OutboundConnectionStatusCodePendingAcceptance,
		types.OutboundConnectionStatusCodeApproved:     types.OutboundConnectionStatusCodeProvisioning,
		types.OutboundConnectionStatusCodeProvisioning: types.OutboundConnectionStatusCodeActive,
		types.OutboundConnectionStatusCodeRejecting:    types.OutboundConnectionStatusCodeRejected,
		types.OutboundConnectionStatusCodeDeleting:     types.OutboundConnectionStatusCodeDeleted,
	}
	to, moves := next[status]
	if !moves || f.hold {
		return
	}
	if f.reads[id]++; f.reads[id] < connSettleReads {
		return
	}
	delete(f.reads, id)
	switch {
	case status == types.OutboundConnectionStatusCodeValidating && !connValid(f.store.Resource(outboundConnectionType, id)):
		f.status[id] = types.OutboundConnectionStatusCodeValidationFailed
	case to == types.OutboundConnectionStatusCodeDeleted:
		f.deleted[id] = f.store.Resource(outboundConnectionType, id)
		f.store.Remove(outboundConnectionType, id)
		delete(f.status, id)
	default:
		f.status[id] = to
	}
}

// accept and reject answer a connection as the destination domain's owner, as AcceptInboundConnection and
// RejectInboundConnection do.
func (f *connFake) accept(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = types.OutboundConnectionStatusCodeApproved
}

func (f *connFake) reject(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[id] = types.OutboundConnectionStatusCodeRejecting
}

// ids returns the connections AWS lists: those in the store, in its order, then the deleted ones, by ID.
func (f *connFake) ids() []string {
	ids := f.store.List(outboundConnectionType)
	for id := range f.deleted {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

func (f *connFake) DescribeOutboundConnections(ctx context.Context, in *opensearch.DescribeOutboundConnectionsInput, _ ...func(*opensearch.Options)) (*opensearch.DescribeOutboundConnectionsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DescribeOutboundConnections"); err != nil {
		return nil, err
	}
	if in.MaxResults > 100 {
		return nil, connFault(connValidationCode, "MaxResults must be at most 100")
	}
	if len(in.Filters) > 0 {
		return nil, connFault(connValidationCode, "the fake doesn't model filters, whose names AWS doesn't document")
	}
	// A token names the read it continues and where, so a read leaves a connection out of every page or none.
	read, start := 0, 0
	if in.NextToken != nil {
		r, offset, found := strings.Cut(aws.ToString(in.NextToken), "-")
		var err error
		if read, err = strconv.Atoi(r); found && err == nil {
			start, err = strconv.Atoi(offset)
		}
		if !found || err != nil || read <= 0 || read > f.read || start <= 0 {
			return nil, connFault("InvalidPaginationTokenException", "the token %q isn't one AWS returned", aws.ToString(in.NextToken))
		}
	} else {
		f.read++
		read = f.read
		f.tick()
	}
	listed := []string{}
	for _, id := range f.ids() {
		if made, ok := f.created[id]; !ok || read > made+f.lagReads {
			listed = append(listed, id)
		}
	}
	end := min(start+connPageSize, len(listed))
	out := &opensearch.DescribeOutboundConnectionsOutput{Connections: []types.OutboundConnection{}}
	for _, id := range listed[min(start, end):end] {
		out.Connections = append(out.Connections, *f.connection(id))
		f.settle(id)
	}
	if end < len(listed) {
		out.NextToken = aws.String(fmt.Sprintf("%d-%d", read, end))
	}
	return out, nil
}

func (f *connFake) DeleteOutboundConnection(ctx context.Context, in *opensearch.DeleteOutboundConnectionInput, _ ...func(*opensearch.Options)) (*opensearch.DeleteOutboundConnectionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin("DeleteOutboundConnection"); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ConnectionId)
	if len(id) < 10 || len(id) > 256 || !connectionIDPattern.MatchString(id) {
		return nil, connFault(connValidationCode, "ConnectionId %q doesn't match the pattern", id)
	}
	p := f.store.Resource(outboundConnectionType, id)
	if p == nil {
		return nil, connFault("ResourceNotFoundException", "the connection %q doesn't exist", id)
	}
	if status := f.statusOf(id); connectionSettling(status) || status == types.OutboundConnectionStatusCodeDeleting {
		return nil, connFault(connBusyCode, "the connection %q is %s", id, status)
	}
	if lo, lr, ln := connDomainValues(p, "LocalDomainInfo"); f.processing[connDomainKey(lo, lr, ln)] > 0 {
		return nil, connFault(connBusyCode, "the domain %q is processing a change", ln)
	}
	f.status[id] = types.OutboundConnectionStatusCodeDeleting
	delete(f.reads, id)
	return &opensearch.DeleteOutboundConnectionOutput{Connection: f.connection(id)}, nil
}
