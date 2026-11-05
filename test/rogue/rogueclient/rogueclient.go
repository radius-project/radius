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

// Package rogueclient implements the rogue-client binary that runs in the rogue pod.
//
//	rogue-client idle
//	rogue-client request [-X method] [-H 'Name: value']... [--data body]
//	    [--cert file --key file] [--cacert file] [-k] [--max-time 30s] URL
//
// request writes the response body to stdout followed by StatusMarker and the HTTP
// status code, or StatusMarker and 0 when no HTTP response was received.
package rogueclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// StatusMarker precedes the HTTP status code on stdout.
	StatusMarker = "\nradius-rogue-status:"

	// ExitOK means an HTTP response was received, whatever its status.
	ExitOK = 0
	// ExitTransport means no HTTP response was received, for example a refused
	// connection or a failed TLS handshake.
	ExitTransport = 1
	// ExitUsage means the arguments were invalid.
	ExitUsage = 2
)

const usage = "usage: rogue-client idle | rogue-client request [flags] URL"

// Run executes the command in args and returns the process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return ExitUsage
	}

	switch args[0] {
	case "idle":
		<-ctx.Done()
		return ExitOK
	case "request":
		return request(ctx, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s\n", args[0], usage)
		return ExitUsage
	}
}

type headers []string

func (h *headers) String() string { return strings.Join(*h, ", ") }

func (h *headers) Set(value string) error {
	*h = append(*h, value)
	return nil
}

type requestOptions struct {
	method   string
	headers  headers
	data     string
	cert     string
	key      string
	caCert   string
	insecure bool
	timeout  time.Duration
	url      string
}

func request(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseRequest(args, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "rogue-client: %v\n", err)
		return ExitUsage
	}

	req, client, err := buildRequest(ctx, opts)
	if err != nil {
		fmt.Fprintf(stderr, "rogue-client: %v\n", err)
		return ExitUsage
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(stderr, "rogue-client: %v\n", err)
		fmt.Fprint(stdout, StatusMarker+"0")
		return ExitTransport
	}
	defer resp.Body.Close()

	if _, err := io.Copy(stdout, resp.Body); err != nil {
		fmt.Fprintf(stderr, "rogue-client: reading response body: %v\n", err)
	}
	fmt.Fprintf(stdout, "%s%d", StatusMarker, resp.StatusCode)
	return ExitOK
}

func parseRequest(args []string, stderr io.Writer) (requestOptions, error) {
	opts := requestOptions{}
	fs := flag.NewFlagSet("request", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.method, "X", http.MethodGet, "HTTP method")
	fs.Var(&opts.headers, "H", "request header as 'Name: value' (repeatable)")
	fs.StringVar(&opts.data, "data", "", "raw request body")
	fs.StringVar(&opts.cert, "cert", "", "client certificate PEM file")
	fs.StringVar(&opts.key, "key", "", "client private key PEM file")
	fs.StringVar(&opts.caCert, "cacert", "", "CA certificate PEM file used to verify the server")
	fs.BoolVar(&opts.insecure, "k", false, "skip server certificate verification")
	fs.DurationVar(&opts.timeout, "max-time", 30*time.Second, "request timeout")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 1 {
		return opts, errors.New("request requires exactly one URL")
	}
	opts.url = fs.Arg(0)
	if (opts.cert == "") != (opts.key == "") {
		return opts, errors.New("--cert and --key must be set together")
	}
	return opts, nil
}

func buildRequest(ctx context.Context, opts requestOptions) (*http.Request, *http.Client, error) {
	var body io.Reader
	if opts.data != "" {
		body = strings.NewReader(opts.data)
	}
	req, err := http.NewRequestWithContext(ctx, opts.method, opts.url, body)
	if err != nil {
		return nil, nil, err
	}
	for _, h := range opts.headers {
		name, value, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, nil, fmt.Errorf("invalid header %q, expected 'Name: value'", h)
		}
		req.Header.Add(strings.TrimSpace(name), strings.TrimSpace(value))
	}

	tlsConfig := &tls.Config{InsecureSkipVerify: opts.insecure} //nolint:gosec // -k deliberately skips verification for negative tests.
	if opts.caCert != "" {
		pem, err := os.ReadFile(opts.caCert)
		if err != nil {
			return nil, nil, fmt.Errorf("reading CA certificate: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, nil, fmt.Errorf("no certificates found in %s", opts.caCert)
		}
		tlsConfig.RootCAs = pool
	}
	if opts.cert != "" {
		cert, err := tls.LoadX509KeyPair(opts.cert, opts.key)
		if err != nil {
			return nil, nil, fmt.Errorf("loading client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return req, &http.Client{Transport: transport, Timeout: opts.timeout}, nil
}
