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

package redshift

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/redshift/types"
	"github.com/aws/aws-sdk-go-v2/service/redshiftserverless"
	serverlesstypes "github.com/aws/aws-sdk-go-v2/service/redshiftserverless/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const registrationType = "AWS::Redshift::NamespaceRegistration"

// RegistrationAPI is the part of the Redshift client NamespaceRegistration uses.
type RegistrationAPI interface {
	RegisterNamespace(ctx context.Context, params *redshift.RegisterNamespaceInput, optFns ...func(*redshift.Options)) (*redshift.RegisterNamespaceOutput, error)
	DeregisterNamespace(ctx context.Context, params *redshift.DeregisterNamespaceInput, optFns ...func(*redshift.Options)) (*redshift.DeregisterNamespaceOutput, error)
	DescribeDataSharesForProducer(ctx context.Context, params *redshift.DescribeDataSharesForProducerInput, optFns ...func(*redshift.Options)) (*redshift.DescribeDataSharesForProducerOutput, error)
	DescribeClusters(ctx context.Context, params *redshift.DescribeClustersInput, optFns ...func(*redshift.Options)) (*redshift.DescribeClustersOutput, error)
}

// RegistrationServerlessAPI is the part of the Redshift Serverless client NamespaceRegistration uses, to find a
// serverless namespace's ARN and whether it and its workgroup are busy.
type RegistrationServerlessAPI interface {
	GetWorkgroup(ctx context.Context, params *redshiftserverless.GetWorkgroupInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.GetWorkgroupOutput, error)
	GetNamespace(ctx context.Context, params *redshiftserverless.GetNamespaceInput, optFns ...func(*redshiftserverless.Options)) (*redshiftserverless.GetNamespaceOutput, error)
}

var (
	_ native.Handler           = (*NamespaceRegistration)(nil)
	_ native.IdentifierEncoder = (*NamespaceRegistration)(nil)

	registrationRegion           = awsapi.InRegion[redshift.Options]
	registrationServerlessRegion = awsapi.InRegion[redshiftserverless.Options]

	// registrationServerlessName is the form of a serverless namespace or workgroup name, and registrationClusterName
	// of a cluster identifier, in lowercase, as AWS keeps them, so one resource has one identifier.
	registrationServerlessName = regexp.MustCompile(`^[a-z0-9-]{3,64}$`)
	registrationClusterName    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	registrationAccount        = regexp.MustCompile(`^[0-9]{12}$`)

	// registrationClusterBusy are the cluster states of a change that ends on its own. A cluster in another state
	// that isn't available, such as storage-full or paused, isn't waited for, because it may stay that way.
	registrationClusterBusy = []string{"creating", "modifying", "rebooting", "renaming", "resizing", "rotating-keys", "updating-hsm", "cancelling-resize"}
)

const (
	// registrationClusterNotFound is the code of a cluster or workgroup AWS doesn't have, registrationBusy of a
	// cluster or namespace that is busy with another change, and registrationNoNamespace of a namespace AWS doesn't
	// have.
	registrationClusterNotFound = "ClusterNotFound"
	registrationBusy            = "InvalidClusterState"
	registrationNoNamespace     = "InvalidNamespaceFault"

	// registrationServerlessNotFound is the code Redshift Serverless returns for a workgroup or namespace it doesn't
	// have.
	registrationServerlessNotFound = "ResourceNotFoundException"

	// registrationConsumerPrefix is what the data share association of a registration puts before the consumer
	// account.
	registrationConsumerPrefix = "DataCatalog/"
)

// NamespaceRegistration serves AWS::Redshift::NamespaceRegistration: a provisioned cluster or a serverless namespace
// registered to the AWS Glue Data Catalog of a consumer account, made with RegisterNamespace. Its identifier is the
// serverless namespace, workgroup, and consumer account joined with "|", or the cluster and consumer account.
//
// AWS has no call that reads a registration. RegisterNamespace makes a data share of type INTERNAL on the producer
// namespace, with an association whose consumer is DataCatalog/ and the consumer account, so the handler reads the
// registration as that association. A registration changes the cluster, or the serverless namespace and workgroup,
// which is busy for a while, so each operation waits for it to be free.
type NamespaceRegistration struct {
	api        RegistrationAPI
	serverless RegistrationServerlessAPI
}

// NewNamespaceRegistration returns the handler for Redshift namespace registrations.
func NewNamespaceRegistration(api RegistrationAPI, serverless RegistrationServerlessAPI) *NamespaceRegistration {
	return &NamespaceRegistration{api: api, serverless: serverless}
}

// TypeName returns AWS::Redshift::NamespaceRegistration.
func (h *NamespaceRegistration) TypeName() string {
	return registrationType
}

// Options returns how UCP uses the handler. AWS accepts a registration it already has, so UCP sends a lost create
// again when it can't find it.
func (h *NamespaceRegistration) Options() native.Options {
	return native.Options{List: native.ListNone, Recovery: native.RecoverByIdentifier}
}

// registration is the namespace and consumer of a registration. Cluster is set for a provisioned cluster, and
// Namespace and Workgroup for a serverless namespace.
type registration struct {
	Namespace string
	Workgroup string
	Cluster   string
	Consumer  string
}

// invalidRegistration is the failure of a template value the handler can't send. It names the property and what it
// must be, never the value.
func invalidRegistration(name string, want string) error {
	return &native.Failure{
		Message:     fmt.Sprintf("%s of an %s must be %s", name, registrationType, want),
		ClientError: true,
		Outcome:     native.OutcomeRejected,
	}
}

// Identifier checks the template's values, so UCP rejects one AWS wouldn't take before any AWS call, and returns the
// identifier: NamespaceIdentifier|WorkgroupIdentifier|consumer for a serverless namespace, and
// ClusterIdentifier|consumer for a cluster.
func (h *NamespaceRegistration) Identifier(properties map[string]any) (string, error) {
	r, complete, err := parseRegistration(properties)
	if err != nil || !complete {
		return "", err
	}
	return r.identifier(), nil
}

func (r registration) identifier() string {
	if r.Cluster != "" {
		return r.Cluster + "|" + r.Consumer
	}
	return r.Namespace + "|" + r.Workgroup + "|" + r.Consumer
}

// properties returns the registration's properties.
func (r registration) properties() map[string]any {
	namespace := map[string]any{"ProvisionedIdentifier": map[string]any{"ClusterIdentifier": r.Cluster}}
	if r.Cluster == "" {
		namespace = map[string]any{"ServerlessIdentifier": map[string]any{"NamespaceIdentifier": r.Namespace, "WorkgroupIdentifier": r.Workgroup}}
	}
	return map[string]any{"NamespaceIdentifier": namespace, "ConsumerIdentifiers": []any{r.Consumer}}
}

// namespaceIdentifier returns the NamespaceIdentifier AWS takes for the registration.
func (r registration) namespaceIdentifier() types.NamespaceIdentifierUnion {
	if r.Cluster != "" {
		return &types.NamespaceIdentifierUnionMemberProvisionedIdentifier{Value: types.ProvisionedIdentifier{ClusterIdentifier: aws.String(r.Cluster)}}
	}
	return &types.NamespaceIdentifierUnionMemberServerlessIdentifier{Value: types.ServerlessIdentifier{
		NamespaceIdentifier: aws.String(r.Namespace),
		WorkgroupIdentifier: aws.String(r.Workgroup),
	}}
}

// parseRegistration returns the registration properties give, and whether they give all of it. A value it can't
// use is a client failure.
func parseRegistration(properties map[string]any) (registration, bool, error) {
	r := registration{}
	complete := true
	if value, ok := properties["ConsumerIdentifiers"]; ok && value != nil {
		list, ok := value.([]any)
		if !ok || len(list) != 1 {
			return r, false, invalidRegistration("ConsumerIdentifiers", "a list of one AWS account ID")
		}
		consumer, ok := list[0].(string)
		if !ok || !registrationAccount.MatchString(consumer) {
			return r, false, invalidRegistration("ConsumerIdentifiers", "a list of one AWS account ID of 12 digits")
		}
		r.Consumer = consumer
	} else {
		complete = false
	}

	value, ok := properties["NamespaceIdentifier"]
	if !ok || value == nil {
		return r, false, nil
	}
	namespace, ok := value.(map[string]any)
	oneOf := "an object with either ServerlessIdentifier or ProvisionedIdentifier"
	if !ok || len(namespace) != 1 {
		return r, false, invalidRegistration("NamespaceIdentifier", oneOf)
	}
	if serverless, ok := namespace["ServerlessIdentifier"]; ok {
		fields, err := registrationFields(serverless, "NamespaceIdentifier/ServerlessIdentifier", "NamespaceIdentifier", "WorkgroupIdentifier")
		if err != nil {
			return r, false, err
		}
		r.Namespace, r.Workgroup = fields[0], fields[1]
		for name, v := range map[string]string{"NamespaceIdentifier": r.Namespace, "WorkgroupIdentifier": r.Workgroup} {
			if !registrationServerlessName.MatchString(v) {
				return r, false, invalidRegistration("NamespaceIdentifier/ServerlessIdentifier/"+name, "3 to 64 lowercase letters, digits, and hyphens")
			}
		}
	} else if provisioned, ok := namespace["ProvisionedIdentifier"]; ok {
		fields, err := registrationFields(provisioned, "NamespaceIdentifier/ProvisionedIdentifier", "ClusterIdentifier")
		if err != nil {
			return r, false, err
		}
		r.Cluster = fields[0]
		if !registrationClusterName.MatchString(r.Cluster) {
			return r, false, invalidRegistration("NamespaceIdentifier/ProvisionedIdentifier/ClusterIdentifier", "a lowercase letter, then up to 62 lowercase letters, digits, and hyphens")
		}
	} else {
		return r, false, invalidRegistration("NamespaceIdentifier", oneOf)
	}
	return r, complete, nil
}

// registrationFields returns the string fields with the names of the object value, which must have exactly them.
func registrationFields(value any, path string, names ...string) ([]string, error) {
	object, ok := value.(map[string]any)
	want := "an object with " + strings.Join(names, " and ")
	if !ok || len(object) != len(names) {
		return nil, invalidRegistration(path, want)
	}
	out := []string{}
	for _, name := range names {
		s, ok := object[name].(string)
		if !ok {
			return nil, invalidRegistration(path, want)
		}
		out = append(out, s)
	}
	return out, nil
}

// splitRegistration returns the registration an identifier names.
func splitRegistration(identifier string) (registration, error) {
	parts := strings.Split(identifier, "|")
	r := registration{}
	switch len(parts) {
	case 2:
		r.Cluster, r.Consumer = parts[0], parts[1]
	case 3:
		r.Namespace, r.Workgroup, r.Consumer = parts[0], parts[1], parts[2]
	}
	_, complete, err := parseRegistration(r.properties())
	if len(parts) < 2 || len(parts) > 3 || err != nil || !complete {
		return registration{}, &native.Failure{
			Message: fmt.Sprintf("the identifier %q of an %s must be NamespaceIdentifier|WorkgroupIdentifier|account for a serverless "+
				"namespace, or ClusterIdentifier|account for a cluster", identifier, registrationType),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return r, nil
}

// registrationBusyFailure is the failure of a change AWS rejected because the cluster or namespace is busy with
// another change that ends on its own. UCP answers it with 409, and the caller deploys again.
func registrationBusyFailure(r registration, err error) error {
	what := "The serverless namespace " + r.Namespace
	if r.Cluster != "" {
		what = "The cluster " + r.Cluster
	}
	return &native.Failure{
		Message: fmt.Sprintf("%s is busy with another change, so AWS changed nothing. Deploy again once it is done, and give "+
			"resources that change it dependsOn on each other: %s", what, err.Error()),
		ClientError: true,
		Conflict:    true,
		Outcome:     native.OutcomeRejected,
	}
}

// registrationState is what AWS reports about a registration and the namespace it belongs to.
type registrationState struct {
	// status is the status of the registration's data share association, or "" when AWS lists none.
	status types.DataShareStatus

	// busy is the state of the cluster, namespace, or workgroup while it is busy with a change, and "" when it is
	// free.
	busy string
}

// registered reports whether AWS has the registration, and pending whether it is still being set up.
func (s registrationState) registered() bool {
	return slices.Contains([]types.DataShareStatus{types.DataShareStatusAuthorized, types.DataShareStatusActive, types.DataShareStatusAvailable}, s.status)
}

func (s registrationState) pending() bool {
	return s.status == types.DataShareStatusPendingAuthorization
}

// state reads the registration. It returns ErrNotFound when AWS has no cluster, workgroup, or namespace it names, or
// when the serverless workgroup belongs to another namespace.
func (h *NamespaceRegistration) state(ctx context.Context, region string, r registration) (registrationState, error) {
	var arn string
	s := registrationState{}
	if r.Cluster != "" {
		out, err := h.api.DescribeClusters(ctx, &redshift.DescribeClustersInput{ClusterIdentifier: aws.String(r.Cluster)}, registrationRegion(region))
		if err != nil {
			return s, awsapi.NotFound(err, registrationClusterNotFound)
		}
		if len(out.Clusters) != 1 {
			return s, native.ErrNotFound
		}
		cluster := out.Clusters[0]
		arn = aws.ToString(cluster.ClusterNamespaceArn)
		if status := aws.ToString(cluster.ClusterStatus); slices.Contains(registrationClusterBusy, status) {
			s.busy = "cluster " + status
		}
	} else {
		var err error
		if arn, s.busy, err = h.serverlessNamespace(ctx, region, r); err != nil {
			return s, err
		}
	}

	pages := redshift.NewDescribeDataSharesForProducerPaginator(h.api, &redshift.DescribeDataSharesForProducerInput{ProducerArn: aws.String(arn)})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx, registrationRegion(region))
		if err != nil {
			return s, awsapi.NotFound(err, registrationNoNamespace)
		}
		for _, share := range page.DataShares {
			if share.DataShareType != types.DataShareTypeInternal || aws.ToString(share.ProducerArn) != arn {
				continue
			}
			for _, association := range share.DataShareAssociations {
				if aws.ToString(association.ConsumerIdentifier) == registrationConsumerPrefix+r.Consumer {
					s.status = association.Status
				}
			}
		}
	}
	return s, nil
}

// serverlessNamespace returns the ARN of the registration's serverless namespace, and the state of the namespace or
// workgroup while one is busy. RegisterNamespace registers the workgroup's namespace whatever NamespaceIdentifier
// names, so a workgroup of another namespace is a registration AWS doesn't have.
func (h *NamespaceRegistration) serverlessNamespace(ctx context.Context, region string, r registration) (string, string, error) {
	workgroup, err := h.serverless.GetWorkgroup(ctx, &redshiftserverless.GetWorkgroupInput{WorkgroupName: aws.String(r.Workgroup)}, registrationServerlessRegion(region))
	if err != nil {
		return "", "", awsapi.NotFound(err, registrationServerlessNotFound)
	}
	if workgroup.Workgroup == nil || aws.ToString(workgroup.Workgroup.NamespaceName) != r.Namespace {
		return "", "", native.ErrNotFound
	}
	namespace, err := h.serverless.GetNamespace(ctx, &redshiftserverless.GetNamespaceInput{NamespaceName: aws.String(r.Namespace)}, registrationServerlessRegion(region))
	if err != nil {
		return "", "", awsapi.NotFound(err, registrationServerlessNotFound)
	}
	if namespace.Namespace == nil {
		return "", "", native.ErrNotFound
	}
	busy := ""
	if status := workgroup.Workgroup.Status; status == serverlesstypes.WorkgroupStatusCreating || status == serverlesstypes.WorkgroupStatusModifying {
		busy = "workgroup " + string(status)
	} else if status := namespace.Namespace.Status; status == serverlesstypes.NamespaceStatusModifying {
		busy = "namespace " + string(status)
	}
	return aws.ToString(namespace.Namespace.NamespaceArn), busy, nil
}

// Read returns the registration's properties, or ErrNotFound when AWS lists no association for it, or only one it
// ended or the consumer rejected.
func (h *NamespaceRegistration) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	r, err := splitRegistration(identifier)
	if err != nil {
		return nil, err
	}
	s, err := h.state(ctx, region, r)
	if err != nil {
		return nil, err
	}
	if !s.registered() && !s.pending() {
		return nil, native.ErrNotFound
	}
	return r.properties(), nil
}

// Create registers the namespace. For a serverless namespace, it first checks that the workgroup belongs to it,
// because AWS registers the workgroup's namespace whatever NamespaceIdentifier names.
func (h *NamespaceRegistration) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	r, complete, err := parseRegistration(request.Properties)
	if err != nil {
		return "", err
	}
	if !complete {
		return "", invalidRegistration("NamespaceIdentifier and ConsumerIdentifiers", "set")
	}
	if r.Cluster == "" {
		if err := h.checkWorkgroup(ctx, request.Region, r); err != nil {
			return "", err
		}
	}
	_, err = h.api.RegisterNamespace(ctx, &redshift.RegisterNamespaceInput{
		NamespaceIdentifier: r.namespaceIdentifier(),
		ConsumerIdentifiers: []string{r.Consumer},
	}, registrationRegion(request.Region))
	if awsapi.HasCode(err, registrationBusy) {
		return "", registrationBusyFailure(r, err)
	}
	if err != nil {
		return "", awsapi.Rejected(err)
	}
	return r.identifier(), nil
}

// checkWorkgroup fails unless the registration's workgroup belongs to its namespace.
func (h *NamespaceRegistration) checkWorkgroup(ctx context.Context, region string, r registration) error {
	out, err := h.serverless.GetWorkgroup(ctx, &redshiftserverless.GetWorkgroupInput{WorkgroupName: aws.String(r.Workgroup)}, registrationServerlessRegion(region))
	if awsapi.HasCode(err, registrationServerlessNotFound) {
		return &native.Failure{
			Message:     fmt.Sprintf("AWS has no Redshift Serverless workgroup %s, so the %s can't be created", r.Workgroup, registrationType),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	if err != nil {
		return err
	}
	if out.Workgroup == nil || aws.ToString(out.Workgroup.NamespaceName) != r.Namespace {
		return &native.Failure{
			Message: fmt.Sprintf("The Redshift Serverless workgroup %s doesn't belong to the namespace %s. Set "+
				"NamespaceIdentifier/ServerlessIdentifier/NamespaceIdentifier to the namespace of the workgroup.", r.Workgroup, r.Namespace),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return nil
}

// Update changes nothing: every property identifies the registration, so UCP rejects a change to any of them before
// it calls the handler.
func (h *NamespaceRegistration) Update(ctx context.Context, request native.UpdateRequest) error {
	return nil
}

// Delete deregisters the namespace. DeregisterNamespace succeeds for a consumer AWS has no registration for, so a
// registration that is already gone is found by Progress.
func (h *NamespaceRegistration) Delete(ctx context.Context, region string, identifier string) error {
	r, err := splitRegistration(identifier)
	if err != nil {
		return err
	}
	if r.Cluster == "" {
		// A workgroup of another namespace with the same name would deregister that namespace.
		if _, _, err := h.serverlessNamespace(ctx, region, r); err != nil {
			return err
		}
	}
	_, err = h.api.DeregisterNamespace(ctx, &redshift.DeregisterNamespaceInput{
		NamespaceIdentifier: r.namespaceIdentifier(),
		ConsumerIdentifiers: []string{r.Consumer},
	}, registrationRegion(region))
	if awsapi.HasCode(err, registrationBusy) {
		return registrationBusyFailure(r, err)
	}
	return awsapi.Rejected(awsapi.NotFound(err, registrationClusterNotFound, registrationNoNamespace))
}

// List isn't supported: AWS lists registrations only as data shares of each namespace.
func (h *NamespaceRegistration) List(ctx context.Context, region string) ([]native.Resource, error) {
	return nil, fmt.Errorf("%s can't be listed", registrationType)
}

// Progress reads the registration. A create waits until AWS lists the registration and the cluster, or the
// serverless namespace and workgroup, is free again. AWS lists a serverless registration only once the workgroup is
// free, so a registration AWS doesn't list is still being made while the cluster or workgroup is busy, and is gone
// once it is free: the next deploy registers it again. A delete waits until AWS no longer lists it and the cluster
// or namespace is free, so the next change to it isn't rejected.
func (h *NamespaceRegistration) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	r, err := splitRegistration(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	s, err := h.state(ctx, region, r)
	if err != nil {
		return native.Progress{}, err
	}
	return s.progress(op)
}

// progress returns the progress of op, given what AWS reports about the registration.
func (s registrationState) progress(op native.Operation) (native.Progress, error) {
	status := string(s.status)
	if op == native.OperationDelete {
		if s.registered() || s.pending() {
			return native.Progress{State: status, Waiting: "deregistered"}, nil
		}
		if s.busy != "" {
			return native.Progress{State: s.busy, Waiting: "available"}, nil
		}
		return native.Progress{}, native.ErrNotFound
	}
	if s.pending() || (s.status == "" && s.busy != "") {
		state := status
		if s.busy != "" {
			state = s.busy
		}
		return native.Progress{State: state, Waiting: string(types.DataShareStatusAuthorized)}, nil
	}
	if !s.registered() {
		return native.Progress{}, native.ErrNotFound
	}
	if s.busy != "" {
		return native.Progress{State: s.busy, Waiting: "available"}, nil
	}
	return native.Progress{Done: true, State: status}, nil
}
