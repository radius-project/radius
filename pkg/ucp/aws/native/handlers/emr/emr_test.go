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

package emr

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/emr"
	"github.com/aws/aws-sdk-go-v2/service/emr/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const testRegion = "us-west-2"

func schema(t *testing.T) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/aws-emr-cluster.json")
	require.NoError(t, err)
	return string(b)
}

// fakes maps each handler the suite makes to its fake API.
type fakes struct {
	mu  sync.Mutex
	api map[native.Handler]*fakeAPI
}

func (f *fakes) add(h native.Handler, api *fakeAPI) native.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api == nil {
		f.api = map[native.Handler]*fakeAPI{}
	}
	f.api[h] = api
	return h
}

func (f *fakes) of(h native.Handler) *fakeAPI {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.api[h]
}

// with returns a copy of properties with changes applied, where a nil value removes the property.
func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

func tags(pairs ...string) []any {
	out := []any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"Key": pairs[i], "Value": pairs[i+1]})
	}
	return out
}

// jsonValue returns v as encoding/json reads it, as UCP passes properties to a handler.
func jsonValue(t *testing.T, v any) map[string]any {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

func instances(coreCount int, extra map[string]any) map[string]any {
	out := map[string]any{
		"MasterInstanceGroup":         map[string]any{"InstanceCount": 1, "InstanceType": "m5.xlarge", "Market": "ON_DEMAND", "Name": "master"},
		"CoreInstanceGroup":           map[string]any{"InstanceCount": coreCount, "InstanceType": "m5.xlarge", "Market": "ON_DEMAND", "Name": "core"},
		"Ec2SubnetId":                 "subnet-0a1",
		"KeepJobFlowAliveWhenNoSteps": true,
		"TerminationProtected":        false,
	}
	maps.Copy(out, extra)
	return out
}

func clusterProperties() map[string]any {
	return map[string]any{
		"Name":                  "app-emr",
		"ReleaseLabel":          "emr-7.1.0",
		"Applications":          []any{map[string]any{"Name": "Spark"}},
		"ServiceRole":           "EMR_DefaultRole",
		"JobFlowRole":           "EMR_EC2_DefaultRole",
		"LogUri":                "s3://app-logs/emr/",
		"LogEncryptionKmsKeyId": "arn:aws:kms:us-west-2:111122223333:key/log-key-one",
		"StepConcurrencyLevel":  1,
		"AutoTerminationPolicy": map[string]any{"IdleTimeout": 3600},
		"Instances":             instances(1, nil),
		"Steps": []any{map[string]any{
			"Name": "setup", "ActionOnFailure": "CONTINUE",
			"HadoopJarStep": map[string]any{"Jar": "command-runner.jar", "Args": []any{"echo", "step-secret-arg"}},
		}},
		"Tags": tags("owner", "", "team", "a"),
	}
}

func Test_Suite_Cluster(t *testing.T) {
	var fs fakes
	cluster := with(clusterProperties(), map[string]any{
		"SecurityConfiguration": "kerberos-config",
		"KerberosAttributes": map[string]any{
			"Realm":                            "EC2.INTERNAL",
			"KdcAdminPassword":                 "kdc-admin-secret",
			"CrossRealmTrustPrincipalPassword": "cross-realm-secret",
			"ADDomainJoinUser":                 "ad-joiner",
			"ADDomainJoinPassword":             "ad-join-secret",
		},
	})
	nativetest.Run(t, nativetest.Suite{
		TypeName: clusterType,
		Schema:   schema(t),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewCluster(api), api)
		},
		IdentifierProperty: "Id",
		// Every write-only property of a cluster is also create-only, so there is no ChangedWriteOnly.
		WriteOnly: []string{
			"LogEncryptionKmsKeyId", "Steps", "KerberosAttributes/KdcAdminPassword",
			"KerberosAttributes/CrossRealmTrustPrincipalPassword", "KerberosAttributes/ADDomainJoinUser",
			"KerberosAttributes/ADDomainJoinPassword",
		},
		Properties:       cluster,
		Changed:          with(cluster, map[string]any{"StepConcurrencyLevel": 2}),
		Removable:        "AutoTerminationPolicy",
		CreateOnlyChange: with(cluster, map[string]any{"ReleaseLabel": "emr-7.2.0"}),
		OtherForm:        map[string]any{"LogUri": "s3n://app-logs/emr"},
		MultiCall: with(cluster, map[string]any{
			"StepConcurrencyLevel": 3,
			"Instances":            instances(2, nil),
			"Tags":                 tags("env", "dev", "team", "b"),
		}),
		Calls: 3,
		FailCall: nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
		RemovedItemField: &nativetest.RemovedItemField{
			Before: with(cluster, map[string]any{"Instances": instances(1, map[string]any{"TaskInstanceGroups": []any{taskGroup(scalingRule)}})}),
			After:  with(cluster, map[string]any{"Instances": instances(1, map[string]any{"TaskInstanceGroups": []any{taskGroup(nil)}})}),
			Gone: func(stored map[string]any) bool {
				return asMap(asList(asMap(stored["Instances"])["TaskInstanceGroups"])[0])["AutoScalingPolicy"] == nil
			},
		},
	})
}

// scalingRule is an automatic scaling policy for an instance group.
var scalingRule = map[string]any{
	"Constraints": map[string]any{"MaxCapacity": 6, "MinCapacity": 1},
	"Rules": []any{map[string]any{
		"Name":   "scale-out",
		"Action": map[string]any{"SimpleScalingPolicyConfiguration": map[string]any{"ScalingAdjustment": 1}},
		"Trigger": map[string]any{"CloudWatchAlarmDefinition": map[string]any{
			"ComparisonOperator": "LESS_THAN", "MetricName": "YARNMemoryAvailablePercentage", "Period": 300, "Threshold": 15,
		}},
	}},
}

// taskGroup returns a task instance group with the automatic scaling policy, or none when policy is nil.
func taskGroup(policy any) map[string]any {
	group := map[string]any{"InstanceCount": 1, "InstanceType": "m5.xlarge", "Name": "task"}
	if policy != nil {
		group["AutoScalingPolicy"] = policy
	}
	return group
}

// create creates a cluster from properties through the handler and returns its ID, once it runs.
func create(t *testing.T, h *Cluster, properties map[string]any) string {
	id, err := h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: jsonValue(t, properties), ClientToken: "token-1"})
	require.NoError(t, err)
	for range 3 {
		progress, err := h.Progress(t.Context(), testRegion, id, native.OperationCreate)
		require.NoError(t, err)
		if progress.Done {
			require.Nil(t, progress.Failure)
			return id
		}
	}
	t.Fatal("the create did not finish")
	return ""
}

// update makes the handler update the cluster to desired, with the patch UCP computes.
func update(t *testing.T, h *Cluster, id string, desired map[string]any, removed ...string) error {
	current, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	desired = jsonValue(t, desired)
	return h.Update(t.Context(), native.UpdateRequest{Region: testRegion, Identifier: id, Current: current, Desired: desired, Patch: patchFor(t, current, desired, removed)})
}

// patchFor returns the patch UCP computes from current to desired. UCP leaves out write-only values that didn't
// change, as the test's are.
func patchFor(t *testing.T, current map[string]any, desired map[string]any, removed []string) jsondiff.Patch {
	a, err := json.Marshal(current)
	require.NoError(t, err)
	b, err := json.Marshal(awsapi.Without(desired, "Steps", "LogEncryptionKmsKeyId", "AdditionalInfo"))
	require.NoError(t, err)
	patch, err := awsoperations.GeneratePatchWithOptions(a, b, []byte(schema(t)), awsoperations.PatchOptions{Removed: removed, Equivalent: equivalent})
	require.NoError(t, err)
	return patch
}

// finish reads the progress of an operation until it is done.
func finish(t *testing.T, h *Cluster, id string, op native.Operation) native.Progress {
	for range 5 {
		progress, err := h.Progress(t.Context(), testRegion, id, op)
		require.NoError(t, err)
		if progress.Done {
			return progress
		}
	}
	t.Fatal("the operation did not finish")
	return native.Progress{}
}

func Test_Read_KerberosPasswords(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	passwords := []string{"kdc-admin-secret", "cross-realm-secret", "ad-join-secret"}
	id := create(t, h, with(clusterProperties(), map[string]any{
		"SecurityConfiguration": "kerberos-config",
		"KerberosAttributes": map[string]any{
			"Realm":                            "EC2.INTERNAL",
			"KdcAdminPassword":                 passwords[0],
			"CrossRealmTrustPrincipalPassword": passwords[1],
			"ADDomainJoinUser":                 "joiner",
			"ADDomainJoinPassword":             passwords[2],
		},
	}))

	read, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Realm": "EC2.INTERNAL"}, read["KerberosAttributes"])
	listed, err := h.List(t.Context(), testRegion)
	require.NoError(t, err)
	for _, v := range []any{read, listed} {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		for _, secret := range append(passwords, "joiner", "log-key-one", "step-secret-arg") {
			require.NotContains(t, string(b), secret)
		}
	}
}

func Test_Schema(t *testing.T) {
	parsed := struct {
		WriteOnly  []string `json:"writeOnlyProperties"`
		CreateOnly []string `json:"createOnlyProperties"`
		Primary    []string `json:"primaryIdentifier"`
	}{}
	require.NoError(t, json.Unmarshal([]byte(schema(t)), &parsed))
	require.Equal(t, []string{"/properties/Id"}, parsed.Primary)
	for _, p := range []string{
		"/properties/KerberosAttributes/ADDomainJoinPassword",
		"/properties/KerberosAttributes/CrossRealmTrustPrincipalPassword",
		"/properties/KerberosAttributes/KdcAdminPassword",
		"/properties/Instances/HadoopVersion",
	} {
		require.Contains(t, parsed.WriteOnly, p)
	}
	require.NotContains(t, parsed.CreateOnly, "/properties/Instances", "EMR can resize instance groups")
}

func Test_Find(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	request := native.CreateRequest{Region: testRegion, Properties: jsonValue(t, clusterProperties()), ClientToken: "token-a"}

	found, err := h.Find(t.Context(), request)
	require.NoError(t, err)
	require.Empty(t, found, "AWS has no cluster")

	other := request
	other.ClientToken = "token-b"
	_, err = h.Create(t.Context(), other)
	require.NoError(t, err)
	renamed := request
	renamed.Properties = with(request.Properties, map[string]any{"Name": "other"})
	_, err = h.Create(t.Context(), renamed)
	require.NoError(t, err)
	found, err = h.Find(t.Context(), request)
	require.NoError(t, err)
	require.Empty(t, found, "a cluster with another operation tag or name is not the lost create")

	first, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	found, err = h.Find(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, first, found)

	_, err = h.Create(t.Context(), request)
	require.NoError(t, err)
	_, err = h.Find(t.Context(), request)
	require.ErrorContains(t, err, "2 EMR clusters that match")

	api.errs["DescribeCluster"] = fault("ThrottlingException", "slow down")
	_, err = h.Find(t.Context(), request)
	require.ErrorContains(t, err, "slow down")
	api.errs["ListClusters"] = fault("AccessDeniedException", "denied")
	_, err = h.Find(t.Context(), request)
	require.ErrorContains(t, err, "denied")
}

func Test_Find_SkipsAClusterThatEnds(t *testing.T) {
	store := awsfake.NewCloudControl()
	api := newFakeAPI(store)
	h := NewCluster(api)
	request := native.CreateRequest{Region: testRegion, Properties: jsonValue(t, clusterProperties()), ClientToken: "token-a"}
	id, err := h.Create(t.Context(), request)
	require.NoError(t, err)
	api.errs["DescribeCluster"] = fault("InvalidRequestException", "Cluster id is not valid.")
	found, err := h.Find(t.Context(), request)
	require.NoError(t, err)
	require.Empty(t, found, "a cluster that is gone when Find describes it is not the lost create")
	require.NotEmpty(t, id)
}

func Test_Update(t *testing.T) {
	tasks := []any{
		map[string]any{"InstanceCount": 1, "InstanceType": "m5.xlarge", "Name": "task-a"},
		map[string]any{"InstanceCount": 2, "InstanceType": "r5.xlarge", "Name": "task-b"},
	}
	base := with(clusterProperties(), map[string]any{"Instances": instances(1, map[string]any{"TaskInstanceGroups": tasks})})

	tests := []struct {
		name    string
		desired map[string]any
		calls   []string
		want    map[string]any
	}{
		{
			name: "resize task groups listed in another order",
			desired: with(base, map[string]any{"Instances": instances(1, map[string]any{"TaskInstanceGroups": []any{
				map[string]any{"InstanceCount": 4, "InstanceType": "r5.xlarge", "Name": "task-b"},
				map[string]any{"InstanceCount": 3, "InstanceType": "m5.xlarge", "Name": "task-a"},
			}})}),
			calls: []string{"ModifyInstanceGroups"},
		},
		{
			name: "termination protection and unhealthy node replacement",
			desired: with(base, map[string]any{"Instances": instances(1, map[string]any{
				"TaskInstanceGroups": tasks, "TerminationProtected": true, "UnhealthyNodeReplacement": true,
			})}),
			calls: []string{"SetTerminationProtection", "SetUnhealthyNodeReplacement"},
		},
		{
			name:    "visibility, managed scaling, and auto-termination",
			desired: with(base, map[string]any{"VisibleToAllUsers": false, "ManagedScalingPolicy": map[string]any{"ComputeLimits": map[string]any{"MaximumCapacityUnits": 4, "MinimumCapacityUnits": 2, "UnitType": "Instances"}}, "AutoTerminationPolicy": map[string]any{"IdleTimeout": 7200}}),
			calls:   []string{"PutAutoTerminationPolicy", "PutManagedScalingPolicy", "SetVisibleToAllUsers"},
		},
		{
			name:    "a removed auto-termination policy and step concurrency",
			desired: with(base, map[string]any{"AutoTerminationPolicy": nil, "StepConcurrencyLevel": nil}),
			calls:   []string{"RemoveAutoTerminationPolicy", "ModifyCluster"},
			want:    map[string]any{"StepConcurrencyLevel": 1.0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			h := NewCluster(api)
			id := create(t, h, base)
			removed := []string{}
			for k := range base {
				if _, ok := tt.desired[k]; !ok {
					removed = append(removed, k)
				}
			}
			api.calls = nil
			require.NoError(t, update(t, h, id, tt.desired, awsoperations.RemovedPaths(jsonValue(t, base), jsonValue(t, tt.desired))...))
			changes := []string{}
			for _, c := range api.calls {
				if !strings.HasPrefix(c, "Describe") && !strings.HasPrefix(c, "List") && !strings.HasPrefix(c, "Get") {
					changes = append(changes, c)
				}
			}
			require.ElementsMatch(t, tt.calls, changes)
			require.Nil(t, finish(t, h, id, native.OperationUpdate).Failure)

			read, err := h.Read(t.Context(), testRegion, id)
			require.NoError(t, err)
			want := tt.want
			if want == nil {
				want = tt.desired
			}
			for k, v := range awsapi.Without(jsonValue(t, want), "Steps", "LogEncryptionKmsKeyId") {
				require.True(t, covers(v, read[k], false) || equivalent(k, v, read[k]), "%s: %v != %v", k, v, read[k])
			}
			for _, k := range removed {
				if k != "StepConcurrencyLevel" {
					require.NotContains(t, read, k)
				}
			}
		})
	}
}

func Test_Update_Unchangeable(t *testing.T) {
	base := clusterProperties()
	tests := []struct {
		name    string
		desired map[string]any
		message string
	}{
		{"key pair", with(base, map[string]any{"Instances": instances(1, map[string]any{"Ec2KeyName": "other"})}), "Instances/Ec2KeyName"},
		{"master group", with(base, map[string]any{"Instances": with(instances(1, nil), map[string]any{"MasterInstanceGroup": map[string]any{"InstanceCount": 3, "InstanceType": "m5.xlarge", "Market": "ON_DEMAND", "Name": "master"}})}), "Instances/MasterInstanceGroup"},
		{"core instance type", with(base, map[string]any{"Instances": with(instances(1, nil), map[string]any{"CoreInstanceGroup": map[string]any{"InstanceCount": 1, "InstanceType": "r5.xlarge", "Market": "ON_DEMAND", "Name": "core"}})}), "Instances/CoreInstanceGroup"},
		{"removed core group", with(base, map[string]any{"Instances": with(instances(1, nil), map[string]any{"CoreInstanceGroup": nil})}), "Instances/CoreInstanceGroup"},
		{"removed core group name", with(base, map[string]any{"Instances": with(instances(1, nil), map[string]any{"CoreInstanceGroup": map[string]any{"InstanceCount": 1, "InstanceType": "m5.xlarge", "Market": "ON_DEMAND"}})}), "Instances/CoreInstanceGroup"},
		{"removed core group market", with(base, map[string]any{"Instances": with(instances(1, nil), map[string]any{"CoreInstanceGroup": map[string]any{"InstanceCount": 1, "InstanceType": "m5.xlarge", "Name": "core"}})}), "Instances/CoreInstanceGroup"},
		{"added task group", with(base, map[string]any{"Instances": instances(1, map[string]any{"TaskInstanceGroups": []any{map[string]any{"InstanceCount": 1, "InstanceType": "m5.xlarge"}}})}), "Instances/TaskInstanceGroups"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			h := NewCluster(api)
			id := create(t, h, base)
			api.calls = nil
			err := update(t, h, id, tt.desired, awsoperations.RemovedPaths(jsonValue(t, base), jsonValue(t, tt.desired))...)
			failure := &native.Failure{}
			require.ErrorAs(t, err, &failure)
			require.Equal(t, native.OutcomeRejected, failure.Outcome)
			require.True(t, failure.ClientError)
			require.Contains(t, failure.Message, tt.message)
			require.NotContains(t, api.calls, "ModifyInstanceGroups", "nothing changes in AWS")
		})
	}

	t.Run("a top-level property UCP lets through", func(t *testing.T) {
		h := NewCluster(newFakeAPI(awsfake.NewCloudControl()))
		id := create(t, h, base)
		err := h.Update(t.Context(), native.UpdateRequest{Region: testRegion, Identifier: id, Current: jsonValue(t, base), Desired: jsonValue(t, base),
			Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/Name", Value: "other"}}})
		require.ErrorContains(t, err, "EMR can't change these properties of a running cluster: Name.")
	})
}

func Test_Progress(t *testing.T) {
	t.Run("a create that ends with errors fails with AWS's reason", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewCluster(api)
		id := create(t, h, clusterProperties())
		api.state[id] = types.ClusterStateTerminating
		progress, err := h.Progress(t.Context(), testRegion, id, native.OperationCreate)
		require.NoError(t, err)
		require.True(t, progress.Done)
		require.Equal(t, "AWS reports the cluster as TERMINATING: Terminated by user request", progress.Failure.Message)
	})

	t.Run("an update waits for instance groups to resize", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewCluster(api)
		id := create(t, h, clusterProperties())
		require.NoError(t, update(t, h, id, with(clusterProperties(), map[string]any{"Instances": instances(3, nil)})))
		progress, err := h.Progress(t.Context(), testRegion, id, native.OperationUpdate)
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "RESIZING", Waiting: "RUNNING"}, progress)
		require.Equal(t, native.Progress{Done: true, State: "WAITING"}, finish(t, h, id, native.OperationUpdate))
	})

	t.Run("a delete waits until the cluster ends", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewCluster(api)
		id := create(t, h, clusterProperties())
		require.NoError(t, h.Delete(t.Context(), testRegion, id))
		progress, err := h.Progress(t.Context(), testRegion, id, native.OperationDelete)
		require.NoError(t, err)
		require.Equal(t, native.Progress{State: "TERMINATING", Waiting: "TERMINATED"}, progress)
		_, err = h.Progress(t.Context(), testRegion, id, native.OperationDelete)
		require.ErrorIs(t, err, native.ErrNotFound)

		_, err = h.Read(t.Context(), testRegion, id)
		require.ErrorIs(t, err, native.ErrNotFound, "a terminated cluster AWS still lists reads as not found")
		require.ErrorIs(t, h.Delete(t.Context(), testRegion, id), native.ErrNotFound)
		require.ErrorIs(t, h.Update(t.Context(), native.UpdateRequest{Region: testRegion, Identifier: id}), native.ErrNotFound)
		listed, err := h.List(t.Context(), testRegion)
		require.NoError(t, err)
		require.Empty(t, listed)
	})

	t.Run("errors", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewCluster(api)
		id := create(t, h, clusterProperties())
		_, err := h.Progress(t.Context(), testRegion, "j-MISSING", native.OperationCreate)
		require.ErrorIs(t, err, native.ErrNotFound)
		api.errs["ListInstanceGroups"] = fault("ThrottlingException", "slow down")
		_, err = h.Progress(t.Context(), testRegion, id, native.OperationUpdate)
		require.ErrorContains(t, err, "slow down")
	})
}

func Test_Delete_TerminationProtected(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	id := create(t, h, with(clusterProperties(), map[string]any{"Instances": instances(1, map[string]any{"TerminationProtected": true})}))
	err := h.Delete(t.Context(), testRegion, id)
	require.True(t, awsapi.HasCode(err, "ValidationException"))
	require.ErrorContains(t, err, "termination protected")
	_, err = h.Read(t.Context(), testRegion, id)
	require.NoError(t, err, "the cluster stays")
}

func Test_InstanceFleets(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	ebs := map[string]any{"EbsOptimized": true, "EbsBlockDeviceConfigs": []any{
		map[string]any{"VolumeSpecification": map[string]any{"SizeInGB": 32, "VolumeType": "gp3"}, "VolumesPerInstance": 2},
	}}
	fleets := map[string]any{
		"MasterInstanceFleet": map[string]any{"Name": "master", "TargetOnDemandCapacity": 1, "InstanceTypeConfigs": []any{
			map[string]any{"InstanceType": "m5.xlarge", "EbsConfiguration": ebs},
		}},
		"CoreInstanceFleet": map[string]any{"Name": "core", "TargetSpotCapacity": 2, "InstanceTypeConfigs": []any{
			map[string]any{"InstanceType": "m5.xlarge", "WeightedCapacity": 1},
			map[string]any{"InstanceType": "m5.2xlarge", "WeightedCapacity": 2},
		}, "LaunchSpecifications": map[string]any{"SpotSpecification": map[string]any{"TimeoutAction": "SWITCH_TO_ON_DEMAND", "TimeoutDurationMinutes": 10}}},
		"TaskInstanceFleets": []any{map[string]any{"Name": "task", "TargetSpotCapacity": 1, "InstanceTypeConfigs": []any{map[string]any{"InstanceType": "c5.xlarge"}}}},
		"Ec2SubnetIds":       []any{"subnet-0a1", "subnet-0b2"},
	}
	properties := with(clusterProperties(), map[string]any{"Instances": fleets})
	id := create(t, h, properties)

	read, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	got := asMap(read["Instances"])
	for k, v := range jsonValue(t, fleets) {
		require.True(t, covers(v, got[k], false), "%s: %v != %v", k, v, got[k])
	}
	require.Equal(t, jsonValue(t, ebs), asMap(asList(asMap(got["MasterInstanceFleet"])["InstanceTypeConfigs"])[0])["EbsConfiguration"])

	api.resizing[id] = true
	progress, err := h.Progress(t.Context(), testRegion, id, native.OperationUpdate)
	require.NoError(t, err)
	require.Equal(t, "RESIZING", progress.State)
	require.True(t, finish(t, h, id, native.OperationUpdate).Done)

	api.errs["ListInstanceFleets"] = fault("ThrottlingException", "slow down")
	_, err = h.Read(t.Context(), testRegion, id)
	require.ErrorContains(t, err, "slow down")
	_, err = h.Progress(t.Context(), testRegion, id, native.OperationUpdate)
	require.ErrorContains(t, err, "slow down")
}

func Test_Read_InstanceGroupDetails(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	scaling := map[string]any{
		"Constraints": map[string]any{"MaxCapacity": 4, "MinCapacity": 1},
		"Rules": []any{map[string]any{
			"Name":   "scale-out",
			"Action": map[string]any{"SimpleScalingPolicyConfiguration": map[string]any{"ScalingAdjustment": 1}},
			"Trigger": map[string]any{"CloudWatchAlarmDefinition": map[string]any{
				"ComparisonOperator": "LESS_THAN", "MetricName": "YARNMemoryAvailablePercentage", "Period": 300, "Threshold": 15,
				"Dimensions": []any{map[string]any{"Key": "JobFlowId", "Value": clusterIDVariable}},
			}},
		}},
	}
	configs := []any{map[string]any{"Classification": "spark", "ConfigurationProperties": map[string]any{"maximizeResourceAllocation": "true"}}}
	tasks := []any{map[string]any{"InstanceCount": 1, "InstanceType": "m5.xlarge", "Name": "task", "AutoScalingPolicy": scaling, "Configurations": configs}}
	properties := with(clusterProperties(), map[string]any{
		"Configurations": configs,
		"Instances": instances(1, map[string]any{
			"TaskInstanceGroups": tasks, "Placement": map[string]any{"AvailabilityZone": "us-west-2a"},
		}),
		"PlacementGroupConfigs":   []any{map[string]any{"InstanceRole": "MASTER"}},
		"MonitoringConfiguration": map[string]any{"CloudWatchLogConfiguration": map[string]any{"Enabled": true}},
		"ManagedScalingPolicy":    map[string]any{"ComputeLimits": map[string]any{"MaximumCapacityUnits": 4, "MinimumCapacityUnits": 2, "UnitType": "Instances"}},
		"AdditionalInfo":          map[string]any{"instanceAwsClientConfiguration": map[string]any{"proxyPort": 8099}},
	})
	id := create(t, h, properties)
	read, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	for _, k := range []string{"Configurations", "Instances", "PlacementGroupConfigs", "MonitoringConfiguration", "ManagedScalingPolicy"} {
		v := jsonValue(t, properties)[k]
		require.True(t, covers(v, read[k], false), "%s: %v != %v", k, v, read[k])
	}
	require.NotContains(t, read, "AdditionalInfo")
	require.Equal(t, `{"instanceAwsClientConfiguration":{"proxyPort":8099}}`, api.store.Resource(clusterType, id)["AdditionalInfo"])
	require.Equal(t, clusterIDVariable, asMap(asList(asMap(asMap(asMap(asList(asMap(asList(asMap(read["Instances"])["TaskInstanceGroups"])[0])["AutoScalingPolicy"].(map[string]any)["Rules"])[0])["Trigger"])["CloudWatchAlarmDefinition"])["Dimensions"])[0])["Value"])
}

func Test_Errors(t *testing.T) {
	for _, method := range []string{"ListInstanceGroups", "GetManagedScalingPolicy", "GetAutoTerminationPolicy", "DescribeCluster"} {
		t.Run("Read when "+method+" fails", func(t *testing.T) {
			api := newFakeAPI(awsfake.NewCloudControl())
			h := NewCluster(api)
			id := create(t, h, clusterProperties())
			api.errs[method] = fault("ThrottlingException", "slow down")
			_, err := h.Read(t.Context(), testRegion, id)
			require.ErrorContains(t, err, "slow down")
			_, err = h.List(t.Context(), testRegion)
			require.ErrorContains(t, err, "slow down")
		})
	}
	t.Run("List when ListClusters fails", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		api.errs["ListClusters"] = fault("AccessDeniedException", "denied")
		_, err := NewCluster(api).List(t.Context(), testRegion)
		require.ErrorContains(t, err, "denied")
	})
	t.Run("Create when AWS rejects it", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		api.errs["RunJobFlow"] = fault("ValidationException", "invalid release label")
		_, err := NewCluster(api).Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: jsonValue(t, clusterProperties()), ClientToken: "t"})
		require.ErrorContains(t, err, "invalid release label")
	})
	t.Run("Create with properties of the wrong type", func(t *testing.T) {
		h := NewCluster(newFakeAPI(awsfake.NewCloudControl()))
		for _, properties := range []map[string]any{
			{"Name": 3},
			{"Name": "a", "Instances": map[string]any{"Ec2KeyName": 3}},
			{"Name": "a", "Instances": map[string]any{"MasterInstanceGroup": map[string]any{"InstanceCount": "one"}}},
			{"Name": "a", "Instances": map[string]any{"MasterInstanceFleet": map[string]any{"Name": 1}}},
			{"Name": "a", "AdditionalInfo": func() {}},
		} {
			_, err := h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: properties, ClientToken: "t"})
			require.Error(t, err)
		}
	})
	t.Run("Update when a call fails", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewCluster(api)
		id := create(t, h, clusterProperties())
		api.errs["ListInstanceGroups"] = fault("ThrottlingException", "slow down")
		current := jsonValue(t, clusterProperties())
		desired := jsonValue(t, with(clusterProperties(), map[string]any{"Instances": instances(2, nil)}))
		patch := jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/Instances/CoreInstanceGroup/InstanceCount", Value: 2}}
		err := h.Update(t.Context(), native.UpdateRequest{Region: testRegion, Identifier: id, Current: current, Desired: desired, Patch: patch})
		require.ErrorContains(t, err, "slow down")

		for _, bad := range []map[string]any{
			{"ManagedScalingPolicy": map[string]any{"ComputeLimits": "none"}},
			{"AutoTerminationPolicy": map[string]any{"IdleTimeout": "never"}},
		} {
			patch := jsondiff.Patch{}
			for k := range bad {
				patch = append(patch, jsondiff.Operation{Type: jsondiff.OperationReplace, Path: "/" + k})
			}
			err := h.Update(t.Context(), native.UpdateRequest{Region: testRegion, Identifier: id, Current: current, Desired: bad, Patch: patch})
			require.Error(t, err)
		}
	})
}

func Test_ChangedKeys(t *testing.T) {
	current := map[string]any{"Ec2KeyName": "a", "TerminationProtected": false}
	desired := map[string]any{"Ec2KeyName": "a", "TerminationProtected": true, "Ec2SubnetId": "subnet-1"}
	patch := jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/Instances"}, {Type: jsondiff.OperationReplace, Path: "/Name"}}
	require.Equal(t, []string{"Ec2SubnetId", "TerminationProtected"}, changedKeys(patch, current, desired))
	patch = jsondiff.Patch{{Type: jsondiff.OperationAdd, Path: "/Instances/a~1b"}}
	require.Equal(t, []string{"a/b"}, changedKeys(patch, current, desired))
}

func Test_Equivalent(t *testing.T) {
	group := func(extra map[string]any) map[string]any {
		return with(map[string]any{"InstanceCount": 1.0, "InstanceType": "m5.xlarge"}, extra)
	}
	tests := []struct {
		name     string
		path     string
		template any
		aws      any
		want     bool
	}{
		{"s3n log location", "LogUri", "s3://logs/emr/", "s3n://logs/emr", true},
		{"another log location", "LogUri", "s3://logs/emr/", "s3n://logs/other/", false},
		{"log location without a scheme", "LogUri", "logs/emr", "logs/emr/", true},
		{"log location of another type", "LogUri", 1.0, "s3://logs", false},
		{"applications with versions and Hadoop", "Applications", []any{map[string]any{"Name": "spark"}}, []any{map[string]any{"Name": "Hadoop", "Version": "3.3"}, map[string]any{"Name": "Spark", "Version": "3.5"}}, true},
		{"a missing application", "Applications", []any{map[string]any{"Name": "Hive"}}, []any{map[string]any{"Name": "Spark"}}, false},
		{"task groups in another order with defaults", "Instances/TaskInstanceGroups",
			[]any{group(map[string]any{"Name": "b"}), group(map[string]any{"Name": "a"})},
			[]any{group(map[string]any{"Name": "a", "Market": "ON_DEMAND"}), group(map[string]any{"Name": "b", "Market": "ON_DEMAND"})}, true},
		{"a task group count", "Instances/TaskInstanceGroups", []any{group(nil)}, []any{group(map[string]any{"InstanceCount": 2.0})}, false},
		{"another number of task groups", "Instances/TaskInstanceGroups", []any{group(nil)}, []any{group(nil), group(nil)}, false},
		{"ordered arguments", "Instances/TaskInstanceGroups", []any{group(map[string]any{"Args": []any{"a", "b"}})}, []any{group(map[string]any{"Args": []any{"b", "a"}})}, false},
		{"an object for an object list", "Configurations", []any{map[string]any{"Classification": "spark"}}, []any{"spark"}, false},
		{"a list of strings", "Instances/Ec2SubnetIds", []any{"a"}, []any{"a"}, false},
		{"tags", "Tags", []any{map[string]any{"Key": "a", "Value": "b"}}, []any{map[string]any{"Key": "a", "Value": "b"}}, false},
		{"an object item that is not an object", "Configurations", []any{map[string]any{"Classification": "spark"}}, []any{map[string]any{"Classification": []any{"spark"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, equivalent(tt.path, tt.template, tt.aws))
		})
	}
	require.Equal(t, "x", lowerNames("x"))
	require.Equal(t, []any{"Spark"}, lowerNames([]any{"Spark"}))
	require.False(t, covers(map[string]any{"a": 1.0}, "a", false))
	require.False(t, equivalent("Configurations", []any{map[string]any{"Classification": "a"}}, []any{map[string]any{"Classification": "a"}, map[string]any{"Classification": "b"}}))
}

func Test_Options(t *testing.T) {
	h := NewCluster(newFakeAPI(awsfake.NewCloudControl()))
	require.Equal(t, clusterType, h.TypeName())
	options := h.Options()
	require.Equal(t, native.RecoverBySearch, options.Recovery)
	require.Equal(t, native.ListRegion, options.List)
	require.Empty(t, options.NameProperty, "AWS assigns the cluster ID")
}

func Test_DescribeWithoutCluster(t *testing.T) {
	h := NewCluster(emptyDescribe{newFakeAPI(awsfake.NewCloudControl())})
	_, err := h.Read(t.Context(), testRegion, "j-1")
	require.ErrorIs(t, err, native.ErrNotFound)
	require.True(t, errors.Is(err, native.ErrNotFound))
}

func Test_UpdateRemovals(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	base := with(clusterProperties(), map[string]any{
		"VisibleToAllUsers":    false,
		"ManagedScalingPolicy": map[string]any{"ComputeLimits": map[string]any{"MaximumCapacityUnits": 4, "MinimumCapacityUnits": 2, "UnitType": "Instances"}},
		"Instances":            instances(1, map[string]any{"UnhealthyNodeReplacement": true}),
		"Tags":                 tags("team", "a", "env", "dev"),
		"AdditionalInfo":       `{"key":"value"}`,
	})
	id := create(t, h, base)
	require.Equal(t, `{"key":"value"}`, api.store.Resource(clusterType, id)["AdditionalInfo"])

	desired := with(base, map[string]any{"VisibleToAllUsers": nil, "ManagedScalingPolicy": nil, "Instances": instances(1, nil), "Tags": tags("team", "a")})
	api.calls = nil
	require.NoError(t, update(t, h, id, desired, awsoperations.RemovedPaths(jsonValue(t, base), jsonValue(t, desired))...))
	require.Subset(t, api.calls, []string{"SetVisibleToAllUsers", "RemoveManagedScalingPolicy", "RemoveTags"})
	require.NotContains(t, api.calls, "SetUnhealthyNodeReplacement", "AWS keeps its value when the template no longer sets it")

	read, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	require.Equal(t, true, read["VisibleToAllUsers"])
	require.NotContains(t, read, "ManagedScalingPolicy")
	require.Equal(t, tags("team", "a"), read["Tags"])

	bad := with(base, map[string]any{"ManagedScalingPolicy": nil, "Instances": instances(1, map[string]any{"CoreInstanceGroup": map[string]any{"InstanceCount": "two", "InstanceType": "m5.xlarge", "Market": "ON_DEMAND", "Name": "core"}})})
	err = update(t, h, id, bad)
	require.ErrorContains(t, err, "Instances/CoreInstanceGroup")
}

func Test_ReadSparseCluster(t *testing.T) {
	h := NewCluster(sparseDescribe{newFakeAPI(awsfake.NewCloudControl())})
	read, err := h.Read(t.Context(), testRegion, "j-1")
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Id": "j-1"}, read)
}

func Test_ListSkipsAClusterThatEnds(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	create(t, h, clusterProperties())
	api.errs["DescribeCluster"] = fault("InvalidRequestException", "Cluster id is not valid.")
	listed, err := h.List(t.Context(), testRegion)
	require.NoError(t, err)
	require.Empty(t, listed)
}

func Test_Conversions(t *testing.T) {
	spec := &types.VolumeSpecification{SizeInGB: aws.Int32(32), VolumeType: aws.String("gp3")}
	require.Equal(t, map[string]any{
		"EbsBlockDeviceConfigs": []any{map[string]any{"VolumeSpecification": spec, "VolumesPerInstance": 2}},
		"EbsOptimized":          (*bool)(nil),
	}, ebsConfiguration([]types.EbsBlockDevice{{VolumeSpecification: spec}, {}, {VolumeSpecification: spec}}, nil))

	policy := &types.AutoScalingPolicyDescription{Rules: []types.ScalingRule{{Name: aws.String("no trigger")}}}
	require.Equal(t, map[string]any{"Constraints": (*types.ScalingConstraints)(nil), "Rules": policy.Rules}, autoScalingPolicy(policy, "j-1"))
	require.Nil(t, withoutCount(func() {}))

	var options emr.Options
	inRegion("eu-west-2")(&options)
	require.Equal(t, "eu-west-2", options.Region)
}

// changes returns the calls of api that change AWS, and clears them.
func changes(api *fakeAPI) []string {
	out := []string{}
	for _, c := range api.calls {
		if !strings.HasPrefix(c, "Describe") && !strings.HasPrefix(c, "List") && !strings.HasPrefix(c, "Get") {
			out = append(out, c)
		}
	}
	api.calls = nil
	return out
}

// setInstances changes the stored cluster's Instances, as EMR's scaling does.
func setInstances(api *fakeAPI, id string, change func(inst map[string]any)) {
	props := api.store.Resource(clusterType, id)
	change(asMap(props["Instances"]))
	api.store.Put(clusterType, id, props)
}

func Test_ScaledCounts(t *testing.T) {
	scaling := map[string]any{"ComputeLimits": map[string]any{"MaximumCapacityUnits": 8, "MinimumCapacityUnits": 2, "UnitType": "Instances"}}
	rule := map[string]any{
		"Constraints": map[string]any{"MaxCapacity": 6, "MinCapacity": 1},
		"Rules": []any{map[string]any{
			"Name":   "scale-out",
			"Action": map[string]any{"SimpleScalingPolicyConfiguration": map[string]any{"ScalingAdjustment": 1}},
			"Trigger": map[string]any{"CloudWatchAlarmDefinition": map[string]any{
				"ComparisonOperator": "LESS_THAN", "MetricName": "YARNMemoryAvailablePercentage", "Period": 300, "Threshold": 15,
			}},
		}},
	}
	task := func(policy any) []any {
		group := map[string]any{"InstanceCount": 1, "InstanceType": "m5.xlarge", "Name": "task"}
		if policy != nil {
			group["AutoScalingPolicy"] = policy
		}
		return []any{group}
	}

	t.Run("managed scaling changes group counts", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewCluster(api)
		base := with(clusterProperties(), map[string]any{"ManagedScalingPolicy": scaling, "Instances": instances(1, map[string]any{"TaskInstanceGroups": task(nil)})})
		id := create(t, h, base)
		setInstances(api, id, func(inst map[string]any) {
			asMap(inst["CoreInstanceGroup"])["InstanceCount"] = 3.0
			asMap(asList(inst["TaskInstanceGroups"])[0])["InstanceCount"] = 4.0
		})
		changes(api)
		require.NoError(t, update(t, h, id, base))
		require.Empty(t, changes(api), "the counts EMR set stay")

		resized := with(base, map[string]any{"Instances": instances(1, map[string]any{"TaskInstanceGroups": []any{
			map[string]any{"InstanceCount": 1, "InstanceType": "r5.xlarge", "Name": "task"},
		}})})
		require.ErrorContains(t, update(t, h, id, resized), "Instances/TaskInstanceGroups", "another instance type still can't change")
	})

	t.Run("automatic scaling changes a group's count", func(t *testing.T) {
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewCluster(api)
		core := map[string]any{"InstanceCount": 1, "InstanceType": "m5.xlarge", "Market": "ON_DEMAND", "Name": "core", "AutoScalingPolicy": rule}
		base := with(clusterProperties(), map[string]any{"Instances": instances(1, map[string]any{"CoreInstanceGroup": core, "TaskInstanceGroups": task(rule)})})
		id := create(t, h, base)
		setInstances(api, id, func(inst map[string]any) {
			asMap(inst["CoreInstanceGroup"])["InstanceCount"] = 3.0
			asMap(asList(inst["TaskInstanceGroups"])[0])["InstanceCount"] = 4.0
		})
		read, err := h.Read(t.Context(), testRegion, id)
		require.NoError(t, err)
		tasks := jsonValue(t, base)["Instances"].(map[string]any)["TaskInstanceGroups"]
		require.True(t, equivalent("Instances/TaskInstanceGroups", tasks, asMap(read["Instances"])["TaskInstanceGroups"]), "a task group's scaled count is no change")
		other := []any{map[string]any{"InstanceCount": 1, "InstanceType": "r5.xlarge", "Name": "task"}}
		require.False(t, equivalent("Instances/TaskInstanceGroups", jsonValue(t, map[string]any{"v": other})["v"], asMap(read["Instances"])["TaskInstanceGroups"]))
		require.False(t, equivalent("Instances/TaskInstanceGroups", "task", asMap(read["Instances"])["TaskInstanceGroups"]))

		changes(api)
		require.NoError(t, update(t, h, id, base))
		require.Empty(t, changes(api), "the counts EMR set stay")
	})

	t.Run("managed scaling changes fleet capacities", func(t *testing.T) {
		fleets := func(spot int, instanceType string) map[string]any {
			return map[string]any{
				"MasterInstanceFleet": map[string]any{"Name": "master", "TargetOnDemandCapacity": 1, "InstanceTypeConfigs": []any{map[string]any{"InstanceType": "m5.xlarge"}}},
				"CoreInstanceFleet":   map[string]any{"Name": "core", "TargetSpotCapacity": spot, "InstanceTypeConfigs": []any{map[string]any{"InstanceType": instanceType}}},
				"TaskInstanceFleets":  []any{map[string]any{"Name": "task", "TargetSpotCapacity": spot, "InstanceTypeConfigs": []any{map[string]any{"InstanceType": instanceType}}}},
				"Ec2SubnetIds":        []any{"subnet-0a1"},
			}
		}
		api := newFakeAPI(awsfake.NewCloudControl())
		h := NewCluster(api)
		base := with(clusterProperties(), map[string]any{"ManagedScalingPolicy": scaling, "Instances": fleets(2, "m5.xlarge")})
		id := create(t, h, base)
		setInstances(api, id, func(inst map[string]any) {
			asMap(inst["CoreInstanceFleet"])["TargetSpotCapacity"] = 6.0
			asMap(asList(inst["TaskInstanceFleets"])[0])["TargetSpotCapacity"] = 5.0
		})
		changes(api)
		require.NoError(t, update(t, h, id, base))
		require.Empty(t, changes(api), "the capacities EMR set stay")

		require.ErrorContains(t, update(t, h, id, with(base, map[string]any{"Instances": fleets(2, "r5.xlarge")})), "Instances/CoreInstanceFleet")

		unnamed := fleets(2, "m5.xlarge")
		delete(asMap(unnamed["CoreInstanceFleet"]), "Name")
		require.ErrorContains(t, update(t, h, id, with(base, map[string]any{"Instances": unnamed}), "Instances/CoreInstanceFleet/Name"), "Instances/CoreInstanceFleet",
			"EMR can't clear a fleet's name")
		uncounted := fleets(2, "m5.xlarge")
		delete(asMap(uncounted["CoreInstanceFleet"]), "TargetSpotCapacity")
		require.NoError(t, update(t, h, id, with(base, map[string]any{"Instances": uncounted}), "Instances/CoreInstanceFleet/TargetSpotCapacity"),
			"managed scaling sets the capacity, so removing it changes nothing")
		unscaled := with(base, map[string]any{"ManagedScalingPolicy": nil})
		require.ErrorContains(t, update(t, h, id, unscaled, "ManagedScalingPolicy/ComputeLimits/MaximumCapacityUnits"), "Instances/CoreInstanceFleet, Instances/TaskInstanceFleets",
			"EMR has no call here to set a fleet's capacity")
	})
}

// Test_RemovedAutoScalingPolicy checks that a template that no longer sets a group's automatic scaling policy removes
// it in AWS, and then sets the template's count, for the core group and a task group. A change inside the policy is
// still rejected.
func Test_RemovedAutoScalingPolicy(t *testing.T) {
	core := func(policy any) map[string]any {
		group := map[string]any{"InstanceCount": 1, "InstanceType": "m5.xlarge", "Market": "ON_DEMAND", "Name": "core"}
		if policy != nil {
			group["AutoScalingPolicy"] = policy
		}
		return group
	}
	cluster := func(corePolicy any, taskPolicy any) map[string]any {
		return with(clusterProperties(), map[string]any{"Instances": instances(1, map[string]any{
			"CoreInstanceGroup":  core(corePolicy),
			"TaskInstanceGroups": []any{taskGroup(taskPolicy)},
		})})
	}

	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	base := cluster(scalingRule, scalingRule)
	id := create(t, h, base)
	setInstances(api, id, func(inst map[string]any) {
		asMap(inst["CoreInstanceGroup"])["InstanceCount"] = 3.0
		asMap(asList(inst["TaskInstanceGroups"])[0])["InstanceCount"] = 4.0
	})
	changes(api)

	changed := jsonValue(t, base)
	rules := asList(asMap(asMap(asMap(changed["Instances"])["CoreInstanceGroup"])["AutoScalingPolicy"])["Rules"])
	asMap(rules[0])["Name"] = "scale-out-faster"
	err := update(t, h, id, changed, awsoperations.RemovedPaths(jsonValue(t, base), changed)...)
	require.ErrorContains(t, err, "Instances/CoreInstanceGroup", "a change inside the policy can't be applied")
	require.Empty(t, changes(api))

	removed := cluster(nil, nil)
	require.NoError(t, update(t, h, id, removed, awsoperations.RemovedPaths(jsonValue(t, base), jsonValue(t, removed))...))
	require.Equal(t, []string{"RemoveAutoScalingPolicy", "RemoveAutoScalingPolicy", "ModifyInstanceGroups"}, changes(api), "the policies go first, then the counts")
	finish(t, h, id, native.OperationUpdate)
	read, err := h.Read(t.Context(), testRegion, id)
	require.NoError(t, err)
	inst := asMap(read["Instances"])
	require.Nil(t, asMap(inst["CoreInstanceGroup"])["AutoScalingPolicy"])
	require.Nil(t, asMap(asList(inst["TaskInstanceGroups"])[0])["AutoScalingPolicy"])
	require.EqualValues(t, 1, asMap(inst["CoreInstanceGroup"])["InstanceCount"], "the template's count applies once the policy is gone")
	require.EqualValues(t, 1, asMap(asList(inst["TaskInstanceGroups"])[0])["InstanceCount"])

	require.NoError(t, update(t, h, id, removed))
	require.Empty(t, changes(api), "the removal is fully applied")
}

// Test_ReservedTags checks that a template can't set the operation tag the handler finds a lost create by: a create
// or an update with it is rejected before any change.
func Test_ReservedTags(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewCluster(api)
	reserved := with(clusterProperties(), map[string]any{"Tags": tags(awsapi.OperationTag, "token-1")})
	_, err := h.Create(t.Context(), native.CreateRequest{Region: testRegion, Properties: jsonValue(t, reserved), ClientToken: "token-1"})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Contains(t, failure.Message, awsapi.OperationTag)
	require.Empty(t, api.calls, "nothing reached AWS")

	id := create(t, h, clusterProperties())
	api.calls = nil
	require.ErrorAs(t, update(t, h, id, reserved), &failure)
	require.Equal(t, native.OutcomeRejected, failure.Outcome)
	require.Empty(t, changes(api), "nothing changed in AWS")
}
