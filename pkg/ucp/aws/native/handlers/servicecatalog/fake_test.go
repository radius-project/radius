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

package servicecatalog

import (
	"context"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// notShared is a portfolio no other account shared with this one. Every other portfolio is shared.
const notShared = "port-notshared"

// fakeAPI is an in-memory Service Catalog API for accepted portfolio shares. It keeps them in an awsfake store, in the
// type's property format, so the suite's Cloud Control sees the same resources. A rejected share is still listed once
// before it is gone.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu        sync.Mutex
	rejecting map[string]bool

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, rejecting: map[string]bool{}, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

func (f *fakeAPI) change(apply func() error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	return f.failer.After()
}

func (f *fakeAPI) AcceptPortfolioShare(ctx context.Context, in *servicecatalog.AcceptPortfolioShareInput, _ ...func(*servicecatalog.Options)) (*servicecatalog.AcceptPortfolioShareOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["AcceptPortfolioShare"]; err != nil {
		return nil, err
	}
	id := aws.ToString(in.PortfolioId)
	return &servicecatalog.AcceptPortfolioShareOutput{}, f.change(func() error {
		if id == notShared || in.PortfolioShareType != types.PortfolioShareTypeImported {
			return fault("ResourceNotFoundException")
		}
		if f.store.Resource(acceptedPortfolioShareType, id) != nil {
			return nil
		}
		props := map[string]any{"PortfolioId": id}
		if in.AcceptLanguage != nil {
			props["AcceptLanguage"] = aws.ToString(in.AcceptLanguage)
		}
		f.store.Put(acceptedPortfolioShareType, id, props)
		return nil
	})
}

func (f *fakeAPI) RejectPortfolioShare(ctx context.Context, in *servicecatalog.RejectPortfolioShareInput, _ ...func(*servicecatalog.Options)) (*servicecatalog.RejectPortfolioShareOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["RejectPortfolioShare"]; err != nil {
		return nil, err
	}
	id := aws.ToString(in.PortfolioId)
	return &servicecatalog.RejectPortfolioShareOutput{}, f.change(func() error {
		if f.store.Resource(acceptedPortfolioShareType, id) == nil || in.PortfolioShareType != types.PortfolioShareTypeImported {
			return fault("ResourceNotFoundException")
		}
		f.rejecting[id] = true
		return nil
	})
}

// ListAcceptedPortfolioShares returns one share per page.
func (f *fakeAPI) ListAcceptedPortfolioShares(ctx context.Context, in *servicecatalog.ListAcceptedPortfolioSharesInput, _ ...func(*servicecatalog.Options)) (*servicecatalog.ListAcceptedPortfolioSharesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ListAcceptedPortfolioShares"]; err != nil {
		return nil, err
	}
	out := &servicecatalog.ListAcceptedPortfolioSharesOutput{}
	if in.PortfolioShareType != types.PortfolioShareTypeImported {
		return out, nil
	}
	ids := f.store.List(acceptedPortfolioShareType)
	start, _ := strconv.Atoi(aws.ToString(in.PageToken))
	if start < len(ids) {
		id := ids[start]
		out.PortfolioDetails = []types.PortfolioDetail{{Id: aws.String(id), DisplayName: aws.String("Shared " + id)}}
		if f.rejecting[id] {
			delete(f.rejecting, id)
			f.store.Remove(acceptedPortfolioShareType, id)
			ids = f.store.List(acceptedPortfolioShareType)
			start--
		}
	}
	if start+1 < len(ids) {
		out.NextPageToken = aws.String(strconv.Itoa(start + 1))
	}
	return out, nil
}
