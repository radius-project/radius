// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/cli/kubernetes"
)

const (
	headerXmsDate                = "x-ms-date"
	headerUserAgent              = "User-Agent"
	headerURLEncoded             = "application/x-www-form-urlencoded"
	headerAuthorization          = "Authorization"
	headerAuxiliaryAuthorization = "x-ms-authorization-auxiliary"
	headerMetadata               = "Metadata"
	headerContentType            = "Content-Type"
	bearerTokenPrefix            = "Bearer "
)

// KubernetesEnvironment represents a Kubernetes Radius environment.
type KubernetesEnvironment struct {
	Name               string `mapstructure:"name" validate:"required"`
	Kind               string `mapstructure:"kind" validate:"required"`
	Context            string `mapstructure:"context" validate:"required"`
	Namespace          string `mapstructure:"namespace" validate:"required"`
	DefaultApplication string `mapstructure:"defaultapplication,omitempty"`

	// We tolerate and allow extra fields - this helps with forwards compat.
	Properties map[string]interface{} `mapstructure:",remain"`
}

func (e *KubernetesEnvironment) GetName() string {
	return e.Name
}

func (e *KubernetesEnvironment) GetKind() string {
	return e.Kind
}

func (e *KubernetesEnvironment) GetDefaultApplication() string {
	return e.DefaultApplication
}

// No Status Link for kubernetes
func (e *KubernetesEnvironment) GetStatusLink() string {
	return ""
}

func (e *KubernetesEnvironment) CreateDeploymentClient(ctx context.Context) (clients.DeploymentClient, error) {
	// azcred := &radclient.AnonymousCredential{}
	// connection := arm.NewConnection("http://localhost:9999", azcred, nil)

	// return &kubernetes.KubernetesDeploymentClient{
	// 	Client:    client,
	// 	Namespace: e.Namespace,
	// }, nil
	client, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}

	return &kubernetes.KubernetesDeploymentClient{
		Client:    client,
		Dynamic:   dynamicClient,
		Typed:     typedClient,
		Namespace: e.Namespace,
	}, nil
}

func (e *KubernetesEnvironment) CreateDiagnosticsClient(ctx context.Context) (clients.DiagnosticsClient, error) {
	client, config, err := kubernetes.CreateTypedClient(e.Context)
	if err != nil {
		return nil, err
	}

	return &kubernetes.KubernetesDiagnosticsClient{
		Client:     client,
		RestConfig: config,
		Namespace:  e.Namespace,
	}, nil
}

func (e *KubernetesEnvironment) CreateManagementClient(ctx context.Context) (clients.ManagementClient, error) {
	client, err := kubernetes.CreateRuntimeClient(e.Context, kubernetes.Scheme)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := kubernetes.CreateDynamicClient(e.Context)
	if err != nil {
		return nil, err
	}
	extensionClient, err := kubernetes.CreateExtensionClient(e.Context)
	if err != nil {
		return nil, err
	}

	restConfig, err := kubernetes.CreateRestConfig(e.Context)
	if err != nil {
		return nil, err
	}

	restClient, err := kubernetes.CreateRestClient(e.Context)
	if err != nil {
		return nil, err
	}

	roundTripper, err := kubernetes.CreateRestRoundTripper(e.Context)
	if err != nil {
		return nil, err
	}

	azcred := &radclient.AnonymousCredential{}

	// 	con := arm.NewDefaultConnection(azcred, nil)
	connection := arm.NewConnection(fmt.Sprintf("%s%s%s", restConfig.Host, restConfig.APIPath, "/apis/api.radius.dev/v1alpha3"), azcred, &arm.ConnectionOptions{
		HTTPClient: &TestClient{Client: roundTripper},
	})
	// connection := arm.NewConnection(fmt.Sprintf("%s%s%s", restConfig.Host, restConfig.APIPath, "/apis/api.radius.dev/v1alpha3"), azcred, &arm.ConnectionOptions{})
	return &kubernetes.KubernetesManagementClient{
		Client:          client,
		DynamicClient:   dynamicClient,
		ExtensionClient: extensionClient,
		Namespace:       e.Namespace,
		EnvironmentName: e.Name,
		RestClient:      restClient,
		Connection:      connection,
		ResourceGroup:   "test", // TODO fill these in with more specific info about env
		SubscriptionID:  "123",
	}, nil
}

var _ policy.Transporter = &TestClient{}

type TestClient struct {
	Client http.RoundTripper
}

func (t *TestClient) Do(req *http.Request) (*http.Response, error) {
	// resp := t.Client.Verb(req.Method).RequestURI(req.RequestURI).Body(req.Body).Do(context.TODO())
	resp, err := t.Client.RoundTrip(req)
	return resp, err
}

var _ azcore.TokenCredential = &K8sToken{}

type K8sToken struct {
	BearerToken string
}

func (k *K8sToken) GetToken(ctx context.Context, options policy.TokenRequestOptions) (*azcore.AccessToken, error) {
	return &azcore.AccessToken{Token: k.BearerToken, ExpiresOn: time.Now().Add(time.Hour)}, nil
}

type bearerTokenPolicy struct {
	// mainResource is the resource to be retreived using the tenant specified in the credential
	mainResource *expiringResource
	// auxResources are additional resources that are required for cross-tenant applications
	auxResources map[string]*expiringResource
	// the following fields are read-only
	creds   azcore.TokenCredential
	options policy.TokenRequestOptions
}

type expiringResource struct {
	// cond is used to synchronize access to the shared resource embodied by the remaining fields
	cond *sync.Cond

	// acquiring indicates that some thread/goroutine is in the process of acquiring/updating the resource
	acquiring bool

	// resource contains the value of the shared resource
	resource interface{}

	// expiration indicates when the shared resource expires; it is 0 if the resource was never acquired
	expiration time.Time

	// acquireResource is the callback function that actually acquires the resource
	acquireResource acquireResource
}

type acquireResource func(state interface{}) (newResource interface{}, newExpiration time.Time, err error)

type acquiringResourceState struct {
	req *policy.Request
	p   bearerTokenPolicy
}

// acquire acquires or updates the resource; only one
// thread/goroutine at a time ever calls this function
func acquire(state interface{}) (newResource interface{}, newExpiration time.Time, err error) {
	s := state.(acquiringResourceState)
	tk, err := s.p.creds.GetToken(s.req.Raw().Context(), s.p.options)
	if err != nil {
		return nil, time.Time{}, err
	}
	return tk, tk.ExpiresOn, nil
}

func newExpiringResource(ar acquireResource) *expiringResource {
	return &expiringResource{cond: sync.NewCond(&sync.Mutex{}), acquireResource: ar}
}

func (er *expiringResource) GetResource(state interface{}) (interface{}, error) {
	// If the resource is expiring within this time window, update it eagerly.
	// This allows other threads/goroutines to keep running by using the not-yet-expired
	// resource value while one thread/goroutine updates the resource.
	const window = 2 * time.Minute // This example updates the resource 2 minutes prior to expiration

	now, acquire, resource := time.Now(), false, er.resource
	// acquire exclusive lock
	er.cond.L.Lock()
	for {
		if er.expiration.IsZero() || er.expiration.Before(now) {
			// The resource was never acquired or has expired
			if !er.acquiring {
				// If another thread/goroutine is not acquiring/updating the resource, this thread/goroutine will do it
				er.acquiring, acquire = true, true
				break
			}
			// Getting here means that this thread/goroutine will wait for the updated resource
		} else if er.expiration.Add(-window).Before(now) {
			// The resource is valid but is expiring within the time window
			if !er.acquiring {
				// If another thread/goroutine is not acquiring/renewing the resource, this thread/goroutine will do it
				er.acquiring, acquire = true, true
				break
			}
			// This thread/goroutine will use the existing resource value while another updates it
			resource = er.resource
			break
		} else {
			// The resource is not close to expiring, this thread/goroutine should use its current value
			resource = er.resource
			break
		}
		// If we get here, wait for the new resource value to be acquired/updated
		er.cond.Wait()
	}
	er.cond.L.Unlock() // Release the lock so no threads/goroutines are blocked

	var err error
	if acquire {
		// This thread/goroutine has been selected to acquire/update the resource
		var expiration time.Time
		resource, expiration, err = er.acquireResource(state)

		// Atomically, update the shared resource's new value & expiration.
		er.cond.L.Lock()
		if err == nil {
			// No error, update resource & expiration
			er.resource, er.expiration = resource, expiration
		}
		er.acquiring = false // Indicate that no thread/goroutine is currently acquiring the resrouce

		// Wake up any waiting threads/goroutines since there is a resource they can ALL use
		er.cond.L.Unlock()
		er.cond.Broadcast()
	}
	return resource, err // Return the resource this thread/goroutine can use
}

// PolicyFunc is a type that implements the Policy interface.
// Use this type when implementing a stateless policy as a first-class function.
type PolicyFunc func(*policy.Request) (*http.Response, error)

func newBearerTokenPolicy(creds azcore.TokenCredential, opts runtime.AuthenticationOptions) *bearerTokenPolicy {
	p := &bearerTokenPolicy{
		creds:        creds,
		options:      opts.TokenRequest,
		mainResource: newExpiringResource(acquire),
	}
	if len(opts.AuxiliaryTenants) > 0 {
		p.auxResources = map[string]*expiringResource{}
	}
	for _, t := range opts.AuxiliaryTenants {
		p.auxResources[t] = newExpiringResource(acquire)

	}
	return p
}

func (b *bearerTokenPolicy) Do(req *policy.Request) (*http.Response, error) {
	as := acquiringResourceState{
		p:   *b,
		req: req,
	}
	tk, err := b.mainResource.GetResource(as)
	if err != nil {
		return nil, err
	}
	if token, ok := tk.(*azcore.AccessToken); ok {
		req.Raw().Header.Set(headerXmsDate, time.Now().UTC().Format(http.TimeFormat))
		req.Raw().Header.Set(headerAuthorization, fmt.Sprintf("Bearer %s", token.Token))
	}
	auxTokens := []string{}
	for tenant, er := range b.auxResources {
		bCopy := *b
		bCopy.options.TenantID = tenant
		auxAS := acquiringResourceState{
			p:   bCopy,
			req: req,
		}
		auxTk, err := er.GetResource(auxAS)
		if err != nil {
			return nil, err
		}
		auxTokens = append(auxTokens, fmt.Sprintf("%s%s", bearerTokenPrefix, auxTk.(*azcore.AccessToken).Token))
	}
	if len(auxTokens) > 0 {
		req.Raw().Header.Set(headerAuxiliaryAuthorization, strings.Join(auxTokens, ", "))
	}
	return req.Next()
}

func (k *K8sToken) NewAuthenticationPolicy(options runtime.AuthenticationOptions) policy.Policy {
	return newBearerTokenPolicy(k, options)
}

// var _ azcore.TokenCredential = &AnonymousCredential{}

// type AnonymousCredential struct {
// }

// // PolicyFunc is a type that implements the Policy interface.
// // Use this type when implementing a stateless policy as a first-class function.
// type PolicyFunc func(*policy.Request) (*http.Response, error)

// // Do implements the Policy interface on PolicyFunc.
// func (pf PolicyFunc) Do(req *policy.Request) (*http.Response, error) {
// 	return pf(req)
// }

// func (*AnonymousCredential) NewAuthenticationPolicy(options runtime.AuthenticationOptions) policy.Policy {
// 	return PolicyFunc(func(req *policy.Request) (*http.Response, error) {
// 		return req.Next()
// 	})
// }

// func (a *AnonymousCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (*azcore.AccessToken, error) {
// 	return nil, nil
// }
