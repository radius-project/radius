// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	historyPageSize  = int64(100)
	historyPageBytes = 1 << 20
)

type historyOptions struct {
	principal *principalKey
	scope     string
	since     *time.Time
	after     int64
	through   int64
}

func historyGeneration(value string) (int64, error) {
	generation, err := strconv.ParseInt(value, 10, 64)
	if err != nil || generation < 1 || generation > maxGeneration || strconv.FormatInt(generation, 10) != value {
		return 0, fmt.Errorf("history generation must be a canonical positive integer")
	}
	return generation, nil
}

func parseHistoryOptions(query url.Values, collection bool) (historyOptions, error) {
	options := historyOptions{}
	for name, values := range query {
		if len(values) != 1 {
			return options, fmt.Errorf("query parameter %s must occur once", name)
		}
		value := values[0]
		switch {
		case name == "api-version":
		case name == "principal" && collection:
			if len(value) > 4096 {
				return options, fmt.Errorf("principal filter exceeds byte bound")
			}
			var principal struct {
				Type    *api.PrincipalType `json:"type"`
				Issuer  *string            `json:"issuer"`
				Subject *string            `json:"subject"`
			}
			decoder := json.NewDecoder(strings.NewReader(value))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&principal); err != nil {
				return options, err
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				return options, fmt.Errorf("principal must contain exactly one JSON object")
			}
			p := &api.Principal{Type: principal.Type, Issuer: principal.Issuer, Subject: principal.Subject}
			if err := validateManagementPrincipal(p); err != nil {
				return options, err
			}
			k, err := key(p)
			if err != nil {
				return options, err
			}
			options.principal = &k
		case name == "scope" && collection:
			if err := validateManagementScope(value, "roleAssignments"); err != nil {
				return options, err
			}
			options.scope = strings.ToLower(value)
		case name == "since" && collection:
			since, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return options, fmt.Errorf("since must be an RFC3339 timestamp")
			}
			options.since = &since
		case name == "$skiptoken" && collection:
			after, through, ok := strings.Cut(value, ":")
			if !ok {
				return options, fmt.Errorf("invalid history continuation")
			}
			var err error
			options.after, err = historyGeneration(after)
			if err != nil {
				return options, err
			}
			options.through, err = historyGeneration(through)
			if err != nil || options.after >= options.through {
				return options, fmt.Errorf("invalid history continuation range")
			}
		default:
			return options, fmt.Errorf("unsupported query parameter %s", name)
		}
	}
	return options, nil
}

func (options historyOptions) matches(event *api.AccessChangeResource) (bool, error) {
	properties := event.Properties
	if properties.Principal == nil || properties.Time == nil || properties.PolicyChange == nil ||
		properties.PolicyChange.Target == nil {
		return false, fmt.Errorf("incomplete committed access history")
	}
	if options.principal != nil {
		actor, err := key(properties.Principal)
		if err != nil {
			return false, err
		}
		if actor != *options.principal {
			return false, nil
		}
	}
	if options.since != nil && properties.Time.Before(*options.since) {
		return false, nil
	}
	return options.scope == "" || within(options.scope, strings.ToLower(*properties.PolicyChange.Target)), nil
}

func historyResource(event *api.AccessChangeResource) *api.AccessChangeResource {
	event.Name = to.Ptr(strconv.FormatInt(*event.Properties.Generation, 10))
	event.Type = to.Ptr("Radius.Core/accessChanges")
	return event
}

func (r *PolicyRuntime) readHistory(ctx context.Context, identity Identity, target managementTarget, options historyOptions, requestURL *url.URL) (any, error) {
	ctx, cancel, err := r.operationContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	var result any
	err = r.inspectPolicy(func(snapshot *PolicySnapshot) error {
		// History is installation-wide. A child-scope grant never discloses
		// other actors or policy, even when a scope filter is supplied.
		if err := snapshot.allows(identity, "Radius.Core/accessChanges/read", "/"); err != nil {
			return &policyMutationError{err}
		}
		if target.name != "" {
			generation, err := historyGeneration(target.name)
			if err != nil {
				return &policyMutationError{err}
			}
			if generation > snapshot.Generation() {
				return errManagementNotFound
			}
			event, err := r.store.committedAccessChange(ctx, generation)
			if err != nil {
				return err
			}
			result = historyResource(event)
			return nil
		}
		through := snapshot.Generation()
		if options.through != 0 {
			if options.through > through {
				return &policyMutationError{fmt.Errorf("history continuation is ahead of the authorized policy generation")}
			}
			through = options.through
		}
		page := api.AuthorizationListAccessChangeResource{Value: []*api.AccessChangeResource{}}
		end := min(through, options.after+historyPageSize)
		cursor, size := options.after, 0
		for generation := options.after + 1; generation <= end; generation++ {
			event, err := r.store.committedAccessChange(ctx, generation)
			if err != nil {
				return err
			}
			matches, err := options.matches(event)
			if err != nil {
				return err
			}
			if matches {
				resource := historyResource(event)
				raw, err := json.Marshal(resource)
				if err != nil {
					return err
				}
				if len(page.Value) > 0 && size+len(raw) > historyPageBytes {
					break
				}
				page.Value = append(page.Value, resource)
				size += len(raw)
			}
			cursor = generation
			// A single valid chunked event can exceed the page budget.
			// Return it alone so paging always makes progress.
			if size >= historyPageBytes {
				break
			}
		}
		if cursor < through {
			next := &url.URL{Path: requestURL.Path}
			query := requestURL.Query()
			query.Set("$skiptoken", strconv.FormatInt(cursor, 10)+":"+strconv.FormatInt(through, 10))
			next.RawQuery = query.Encode()
			page.NextLink = to.Ptr(next.String())
		}
		result = page
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func serveHistory(w http.ResponseWriter, r *http.Request, runtime *PolicyRuntime, target managementTarget, routeErr error) {
	identity, verified := IdentityFromContext(r.Context())
	if !verified {
		writeInspectionError(w, r, http.StatusUnauthorized, "InvalidAuthenticationInfo", "A verified Kubernetes caller identity is required.")
		return
	}
	if err := runtime.Ready(); err != nil {
		writeAuthorizationUnavailable(w, r)
		return
	}
	if routeErr != nil {
		writeManagementError(w, r, &policyMutationError{routeErr})
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query["api-version"]) != 1 || query.Get("api-version") != "2025-08-01-preview" {
		writeInspectionError(w, r, http.StatusBadRequest, "InvalidApiVersionParameter", "Use one api-version=2025-08-01-preview.")
		return
	}
	options, err := parseHistoryOptions(query, target.name == "")
	if err != nil {
		writeManagementError(w, r, &policyMutationError{err})
		return
	}
	result, err := runtime.readHistory(r.Context(), identity, target, options, r.URL)
	if err != nil {
		writeManagementError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		ucplog.FromContextOrDiscard(r.Context()).Error(err, "Failed to write access history response")
	}
}
