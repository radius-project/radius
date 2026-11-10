/*
Copyright 2026 The Radius Authors.

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

package helm

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/httpstream"
	streamspdy "k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
)

type proofForwardDialer struct {
	context context.Context
	client  *http.Client
	url     *url.URL
}

// Use the standard authenticated HTTP transport for the upgrade. The client-go
// SPDY round tripper reads upgrade headers directly from a socket without
// honoring cancellation once connected, which can leave stop waiting forever.
func (d proofForwardDialer) Dial(protocols ...string) (httpstream.Connection, string, error) {
	var socket net.Conn
	ctx := httptrace.WithClientTrace(d.context, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { socket = info.Conn },
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url.String(), nil)
	if err != nil {
		return nil, "", errors.New("cannot construct pod proof port-forward request")
	}
	request.Header.Set(httpstream.HeaderConnection, httpstream.HeaderUpgrade)
	request.Header.Set(httpstream.HeaderUpgrade, "SPDY/3.1")
	for _, protocol := range protocols {
		request.Header.Add(httpstream.HeaderProtocolVersion, protocol)
	}
	response, err := d.client.Do(request)
	if err != nil {
		return nil, "", errors.New("cannot establish authenticated pod proof port-forward")
	}
	upgrade := false
	for _, token := range strings.Split(response.Header.Get(httpstream.HeaderConnection), ",") {
		upgrade = upgrade || strings.EqualFold(strings.TrimSpace(token), httpstream.HeaderUpgrade)
	}
	if response.StatusCode != http.StatusSwitchingProtocols || !upgrade ||
		!strings.EqualFold(response.Header.Get(httpstream.HeaderUpgrade), "SPDY/3.1") ||
		response.Header.Get(httpstream.HeaderProtocolVersion) == "" {
		_ = response.Body.Close()
		return nil, "", errors.New("API server rejected pod proof port-forward upgrade")
	}
	body, ok := response.Body.(io.ReadWriteCloser)
	if !ok || socket == nil {
		_ = response.Body.Close()
		return nil, "", errors.New("Kubernetes transport does not support a bidirectional pod proof upgrade")
	}
	connection, err := streamspdy.NewClientConnectionWithPings(&proofUpgradeConnection{Conn: socket, body: body}, 5*time.Second)
	if err != nil {
		_ = body.Close()
		return nil, "", errors.New("cannot initialize pod proof SPDY connection")
	}
	return connection, response.Header.Get(httpstream.HeaderProtocolVersion), nil
}

// Preserve any bytes buffered by net/http while retaining socket deadlines.
type proofUpgradeConnection struct {
	net.Conn
	body io.ReadWriteCloser
}

func (c *proofUpgradeConnection) Read(p []byte) (int, error)  { return c.body.Read(p) }
func (c *proofUpgradeConnection) Write(p []byte) (int, error) { return c.body.Write(p) }
func (c *proofUpgradeConnection) Close() error                { return c.body.Close() }

// openPodProofPortForward authenticates only the API-server tunnel. Callers must
// independently verify the pod/process identity and use a credential-free client
// for public proof requests to the returned loopback origin.
func openPodProofPortForward(ctx context.Context, config *rest.Config, target TLSProofTarget) (string, func(), error) {
	if config == nil || len(validation.IsDNS1123Label(target.Namespace)) != 0 ||
		len(validation.IsDNS1123Subdomain(target.PodName)) != 0 || target.Port == 0 {
		return "", nil, errors.New("invalid pod proof port-forward target")
	}
	config = rest.CopyConfig(config)
	config.Timeout = 10 * time.Second
	config.NextProtos = []string{"http/1.1"}
	transport, err := rest.HTTPClientFor(config)
	if err != nil {
		return "", nil, errors.New("cannot initialize authenticated pod proof port-forward")
	}
	// Client.Timeout wraps response bodies as read-only. The context below
	// bounds the upgrade and tunnel without hiding the upgraded writer.
	transport.Timeout = 0
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", nil, errors.New("cannot initialize pod proof Kubernetes client")
	}
	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	url := client.CoreV1().RESTClient().Post().Namespace(target.Namespace).Resource("pods").
		Name(target.PodName).SubResource("portforward").URL()
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	dialer := proofForwardDialer{context: ctx, client: transport, url: url}
	ready := make(chan struct{})
	// The forwarder otherwise logs raw remote error-stream contents globally.
	// Surface only the sanitized tunnel/observation errors returned to the caller.
	forwarder, err := portforward.NewOnAddressesWithContext(logr.NewContext(ctx, logr.Discard()),
		dialer, []string{"127.0.0.1"}, []string{"0:" + strconv.Itoa(int(target.Port))}, ready, io.Discard, io.Discard)
	if err != nil {
		cancel()
		return "", nil, errors.New("cannot prepare pod proof port-forward")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Readiness or completion below carries the result. Raw upstream errors
		// can contain response bodies and must not become user-facing diagnostics.
		_ = forwarder.ForwardPorts()
	}()
	stop := sync.OnceFunc(func() { cancel(); <-done })
	select {
	case <-ctx.Done():
		stop()
		return "", nil, ctx.Err()
	case <-done:
		contextErr := ctx.Err()
		stop()
		if contextErr != nil {
			return "", nil, contextErr
		}
		return "", nil, errors.New("authenticated pod proof port-forward failed; check API connectivity and create pods/portforward permission")
	case <-ready:
		ports, err := forwarder.GetPorts()
		if err != nil || len(ports) != 1 || ports[0].Local == 0 {
			stop()
			return "", nil, errors.New("pod proof port-forward has no ready port")
		}
		return "http://127.0.0.1:" + strconv.Itoa(int(ports[0].Local)), stop, nil
	}
}
