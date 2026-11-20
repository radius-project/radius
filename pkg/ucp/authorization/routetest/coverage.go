// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

// Package routetest checks authorization mapping against live UCP router trees.
// It is a test helper, not authorization middleware.
package routetest

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/stretchr/testify/require"
)

var parameter = regexp.MustCompile(`\{[^}]+\}`)

// AssertMapped walks every registered route, not just hand-picked route tests.
// Catch-all dispatchers are explicitly reviewed here; concrete requests through
// the resource proxies must still map through the catalog at enforcement time.
func AssertMapped(t *testing.T, router chi.Router, pathBase string) {
	t.Helper()
	catalog, err := authorization.NewCatalog(nil)
	require.NoError(t, err)
	err = chi.Walk(router, func(method, route string, handler http.Handler, middleware ...func(http.Handler) http.Handler) error {
		route = strings.TrimPrefix(route, pathBase)
		route = strings.ReplaceAll(route, "/*/", "/")
		route = strings.TrimSuffix(route, "/")
		if route == "" {
			route = "/"
		}
		if strings.Contains(route, "*") {
			switch route {
			case "/planes/{planeType}/*":
				// Root dispatcher selects a module. All three modules run this
				// same coverage check on their own live router.
				return nil
			case "/planes/azure/{planeName}/*",
				"/planes/radius/{planeName}/providers/*",
				"/planes/radius/{planeName}/resourcegroups/{resourceGroupName}/providers/*":
				// Resource proxies are not grants: malformed paths and unknown
				// permissions are rejected by MapRequest.
				return nil
			default:
				t.Errorf("unreviewed UCP catch-all route: %s %s", method, route)
				return nil
			}
		}
		sample := strings.ReplaceAll(route, "{planeType}", "radius")
		sample = strings.ReplaceAll(sample, "{providerNamespace}", "AWS.S3")
		sample = parameter.ReplaceAllString(sample, "sample")
		mapping, err := catalog.MapRequest(method, sample)
		require.NoError(t, err, "unmapped live UCP route %s %s (sample %s)", method, route, sample)
		require.NotEmpty(t, mapping.Kind, "missing explicit mapping kind for %s %s", method, route)
		if mapping.Kind == authorization.ResourceRequest || mapping.Kind == authorization.ListRequest ||
			mapping.Kind == authorization.MixedResourceListRequest {
			require.NotEmpty(t, mapping.Checks, "resource routes must have checks")
		}
		return nil
	})
	require.NoError(t, err)
}
