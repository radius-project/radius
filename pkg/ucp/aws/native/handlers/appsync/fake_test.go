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

package appsync

import (
	"context"
	"io"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/appsync"
	"github.com/aws/aws-sdk-go-v2/service/appsync/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// fakeAPI is an in-memory AppSync API and S3 bucket. It treats an API and its schema as one resource, an entry in an
// awsfake store keyed by the API ID, in the schema type's property format, so the suite's Cloud Control sees the same
// resources and removing the entry removes the API. StartSchemaCreation accepts the API IDs in apis, the APIs that
// exist. A schema creation is PROCESSING for one read, then SUCCESS, or FAILED for a definition that contains
// "invalid".
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu      sync.Mutex
	apis    map[string]bool
	status  map[string]types.SchemaStatus
	objects map[string]string
	calls   []string

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var (
	_ API       = (*fakeAPI)(nil)
	_ ObjectAPI = (*fakeAPI)(nil)
)

func newFakeAPI(store *awsfake.CloudControl, apis ...string) *fakeAPI {
	f := &fakeAPI{store: store, apis: map[string]bool{}, status: map[string]types.SchemaStatus{}, objects: map[string]string{}, errs: map[string]error{}}
	for _, id := range apis {
		f.apis[id] = true
	}
	return f
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// call records a call and the region its options set.
func (f *fakeAPI) call(name string, region string) error {
	f.calls = append(f.calls, name+" "+region)
	return f.errs[name]
}

func appsyncRegion(optFns []func(*appsync.Options)) string {
	o := appsync.Options{}
	for _, fn := range optFns {
		fn(&o)
	}
	return o.Region
}

func (f *fakeAPI) StartSchemaCreation(ctx context.Context, in *appsync.StartSchemaCreationInput, optFns ...func(*appsync.Options)) (*appsync.StartSchemaCreationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("StartSchemaCreation", appsyncRegion(optFns)); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ApiId)
	if !f.apis[id] {
		return nil, fault("NotFoundException")
	}
	if f.status[id] == types.SchemaStatusProcessing {
		return nil, fault("ConcurrentModificationException")
	}
	if err := f.failer.Before(); err != nil {
		return nil, err
	}
	definition := string(in.Definition)
	f.store.Put(schemaType, id, map[string]any{"ApiId": id, "Definition": definition})
	f.status[id] = types.SchemaStatusProcessing
	if strings.Contains(definition, "invalid") {
		f.status[id] = types.SchemaStatusFailed
	}
	return &appsync.StartSchemaCreationOutput{Status: types.SchemaStatusProcessing}, f.failer.After()
}

func (f *fakeAPI) GetSchemaCreationStatus(ctx context.Context, in *appsync.GetSchemaCreationStatusInput, optFns ...func(*appsync.Options)) (*appsync.GetSchemaCreationStatusOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("GetSchemaCreationStatus", appsyncRegion(optFns)); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ApiId)
	if f.store.Resource(schemaType, id) == nil {
		return nil, fault("NotFoundException")
	}
	switch status := f.status[id]; status {
	case types.SchemaStatusProcessing:
		delete(f.status, id)
		return &appsync.GetSchemaCreationStatusOutput{Status: status}, nil
	case types.SchemaStatusFailed:
		return &appsync.GetSchemaCreationStatusOutput{Status: status, Details: aws.String("Syntax Error")}, nil
	}
	return &appsync.GetSchemaCreationStatusOutput{Status: types.SchemaStatusSuccess}, nil
}

func (f *fakeAPI) GetGraphqlApi(ctx context.Context, in *appsync.GetGraphqlApiInput, optFns ...func(*appsync.Options)) (*appsync.GetGraphqlApiOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.call("GetGraphqlApi", appsyncRegion(optFns)); err != nil {
		return nil, err
	}
	id := aws.ToString(in.ApiId)
	if f.store.Resource(schemaType, id) == nil {
		return nil, fault("NotFoundException")
	}
	return &appsync.GetGraphqlApiOutput{GraphqlApi: &types.GraphqlApi{ApiId: aws.String(id), Name: aws.String("api-" + id)}}, nil
}

func (f *fakeAPI) GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := s3.Options{}
	for _, fn := range optFns {
		fn(&o)
	}
	if err := f.call("GetObject", o.Region); err != nil {
		return nil, err
	}
	body, ok := f.objects[aws.ToString(in.Bucket)+"/"+aws.ToString(in.Key)]
	if !ok {
		return nil, fault("NoSuchKey")
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(body))}, nil
}
