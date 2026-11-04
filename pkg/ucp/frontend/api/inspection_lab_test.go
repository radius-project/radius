//go:build !windows

// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package api

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database"
	authapi "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
)

type inspectionLabDatabase struct {
	database.Client
	fail atomic.Bool
}

func (d *inspectionLabDatabase) Get(ctx context.Context, id string, opts ...database.GetOptions) (*database.Object, error) {
	if d.fail.Load() {
		return nil, errors.New("manual lab database read failure")
	}
	return d.Client.Get(ctx, id, opts...)
}

// TestManualUserAuthorizationLab is deliberately opt-in and compiled only into
// test binaries. Its fixture verifier and stdin controls are not UCP APIs.
func TestManualUserAuthorizationLab(t *testing.T) {
	directory := os.Getenv("RADIUS_USER_AUTHZ_LAB_DIR")
	if directory == "" {
		t.Skip("interactive lab: use bash hack/authz/user-lab.sh")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	info, err := os.Stat(directory)
	require.NoError(t, err)
	require.True(t, info.IsDir() && info.Mode().Perm() == 0700, "lab directory must be private (0700)")
	mode := authapi.UserAuthorizationMode(os.Getenv("RADIUS_USER_AUTHZ_LAB_MODE"))
	require.Contains(t, []authapi.UserAuthorizationMode{
		authapi.UserAuthorizationModeDisabled, authapi.UserAuthorizationModeDryRun,
		authapi.UserAuthorizationModeEnforce, "Unconfigured",
	}, mode)
	missing := os.Getenv("RADIUS_USER_AUTHZ_LAB_MISSING_ANCHOR") == "true"
	require.False(t, mode == "Unconfigured" && missing, "missing anchor requires a configured runtime")
	var db *inspectionLabDatabase
	var store *authorization.PolicyStore
	var runtime *authorization.PolicyRuntime
	var cancel context.CancelFunc
	fixture := startInspectionAggregation(t, directory, func(client database.Client) *authorization.PolicyRuntime {
		if mode == "Unconfigured" {
			return nil
		}
		db = &inspectionLabDatabase{Client: client}
		catalog, err := authorization.NewCatalog(nil)
		require.NoError(t, err)
		store, err = authorization.NewPolicyStore(db, catalog, nil, nil)
		require.NoError(t, err)
		if !missing {
			frontendRequestSeed(t, store, mode)
		}
		runtime, err = authorization.NewPolicyRuntime(store, nil)
		require.NoError(t, err)
		ctx, stop := context.WithCancel(context.Background())
		cancel = stop
		done := make(chan error, 1)
		go func() { done <- runtime.Run(ctx) }()
		t.Cleanup(func() {
			cancel()
			require.ErrorIs(t, <-done, context.Canceled)
		})
		if !missing {
			require.Eventually(t, func() bool { return runtime.Ready() == nil }, time.Second, time.Millisecond)
			aggregationReaderGrants(t, runtime)
		}
		return runtime
	})
	for i, identity := range fixture.identities {
		name := []string{"alice", "bob", "serviceaccount"}[i]
		require.NoError(t, clientcmd.WriteToFile(aggregationKubeconfig(identity.config), filepath.Join(directory, name+".kubeconfig")))
		const config = `{"workspaces":{"default":"isolated","items":{"isolated":{"connection":{"kind":"kubernetes","context":"isolated"},"scope":"/planes/radius/local/resourceGroups/test"}}}}`
		require.NoError(t, os.WriteFile(filepath.Join(directory, name+".rad.json"), []byte(config), 0600))
		if identity.config.BearerToken == "" {
			require.NoError(t, os.WriteFile(filepath.Join(directory, name+".crt"), identity.config.CertData, 0600))
			require.NoError(t, os.WriteFile(filepath.Join(directory, name+".key"), identity.config.KeyData, 0600))
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(directory, "ucp-ca.crt"), fixture.caBundle, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "kube-ca.crt"), fixture.identities[0].config.CAData, 0600))
	_, untrusted := frontendProxyCertificate(t, "front-proxy-client")
	writeLabCertificate(t, directory, "untrusted", untrusted.Certificate[0], untrusted.PrivateKey)
	shellQuote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
	env := fmt.Sprintf("export LAB=%s\nexport UCP=%s\nexport KUBE_API=%s\nexport API='/apis/api.ucp.dev/v1alpha3'\nexport VERSION='2025-08-01-preview'\nexport RAD=%s\n",
		shellQuote(directory), shellQuote(fixture.ucp.URL), shellQuote(strings.TrimSuffix(fixture.identities[0].config.Host, "/")), shellQuote(os.Getenv("RAD_BIN")))
	env += `export KUBECONFIG="$LAB/bob.kubeconfig"
lab_rad() {
    KUBECONFIG="$LAB/${1}.kubeconfig" \
        "$RAD" --config "$LAB/${1}.rad.json" "${@:2}"
}
lab_curl() {
    curl --silent --show-error --max-time 10 --cacert "$LAB/ucp-ca.crt" \
        --cert "$LAB/proxy.crt" --key "$LAB/proxy.key" "$@"
}
lab_api() {
    local identity="$1"
    shift
    curl --silent --show-error --max-time 10 --cacert "$LAB/kube-ca.crt" \
        --cert "$LAB/$identity.crt" --key "$LAB/$identity.key" "$@"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(directory, "env.sh"), []byte(env), 0600))
	fmt.Printf("LAB READY mode=%s; test-only credentials expire within one hour.\nSecond terminal:\n  source %s\n  lab_rad bob auth whoami -o json\nLocal fixture commands: state, fail-reads, resume-reads, initialize, bump, cancel, events, quit\n",
		mode, shellQuote(filepath.Join(directory, "env.sh")))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "ready"), []byte("ready\n"), 0600))
	printState := func() {
		state := runtime.State()
		fmt.Printf("state=%s running=%t generation=%d lastKnownMode=%s asOf=%s\n",
			state.Status, state.Running, state.Generation, state.LastKnownMode, state.AsOf.UTC().Format(time.RFC3339))
	}
	printState()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	offset := 0
	bumps := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := os.ReadFile(filepath.Join(directory, "commands"))
			require.NoError(t, err)
			require.GreaterOrEqual(t, len(data), offset, "control file must be append-only")
			end := strings.IndexByte(string(data[offset:]), '\n')
			if end < 0 {
				continue
			}
			command := strings.TrimSpace(string(data[offset : offset+end]))
			offset += end + 1
			if command == "quit" {
				return
			}
			switch command {
			case "", "state":
			case "events":
				for _, event := range fixture.logs.TakeAll() {
					if event.Message != "User authorization request" {
						continue
					}
					// Print only the observer's bounded metadata, never arbitrary
					// request/response logs, bodies, headers, tokens or keys.
					fields := event.ContextMap()
					safe := map[string]any{}
					for _, key := range []string{"result", "mode", "generation", "coverage", "method", "permission", "target", "reason", "authenticated"} {
						if value, ok := fields[key]; ok {
							safe[key] = value
						}
					}
					data, err := json.Marshal(safe)
					require.NoError(t, err)
					fmt.Println(string(data))
				}
			case "fail-reads", "resume-reads":
				if db == nil {
					fmt.Println("no configured runtime")
					break
				}
				db.fail.Store(command == "fail-reads")
			case "initialize":
				if !missing {
					fmt.Println("anchor already initialized or runtime unconfigured")
					break
				}
				frontendRequestSeed(t, store, mode)
				require.Eventually(t, func() bool { return runtime.Ready() == nil }, 10*time.Second, 50*time.Millisecond)
				aggregationReaderGrants(t, runtime)
				missing = false
			case "bump":
				if runtime == nil {
					fmt.Println("no configured runtime")
					break
				}
				bumps++
				group := fmt.Sprintf("manual-%d", bumps)
				err := runtime.Mutate(t.Context(), aggregationActor(group), authorization.PolicyMutation{
					Target: "/providers/Radius.Core/roleAssignments/" + group,
					Assignment: &authapi.RoleAssignmentProperties{
						Principal:        &authapi.Principal{Type: to.Ptr(authapi.PrincipalTypeGroup), Issuer: to.Ptr("kubernetes:test-cluster"), Subject: &group},
						RoleDefinitionID: to.Ptr("/providers/Radius.Core/roleDefinitions/reader"), Scope: to.Ptr("/"),
					},
				})
				if err != nil {
					fmt.Println("fixture mutation rejected:", err)
				}
			case "cancel":
				if cancel != nil {
					cancel()
				}
			default:
				fmt.Println("unknown command; use state, fail-reads, resume-reads, initialize, bump, cancel, events or quit")
			}
			printState()
		}
	}

}

func writeLabCertificate(t *testing.T, directory, name string, der []byte, key any) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(directory, name+".crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, name+".key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0600))
}
