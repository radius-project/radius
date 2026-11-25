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

package databaseprovider

import (
	context "context"
	"crypto/tls"
	"errors"
	"fmt"
	neturl "net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	store "github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/apiserverstore"
	ucpv1alpha1 "github.com/radius-project/radius/pkg/components/database/apiserverstore/api/ucp.dev/v1alpha1"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/components/database/postgres"
	"github.com/radius-project/radius/pkg/kubeutil"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type databaseClientFactoryFunc func(ctx context.Context, options Options) (store.Client, error)

var databaseClientFactory = map[DatabaseProviderType]databaseClientFactoryFunc{
	TypeAPIServer:  initAPIServerClient,
	TypeInMemory:   initInMemoryClient,
	TypePostgreSQL: initPostgreSQLClient,
}

func initAPIServerClient(ctx context.Context, opt Options) (store.Client, error) {
	if opt.APIServer.Namespace == "" {
		return nil, errors.New("failed to initialize APIServer client: namespace is required")
	}

	cfg, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
		ContextName: opt.APIServer.Context,
		QPS:         kubeutil.DefaultServerQPS,
		Burst:       kubeutil.DefaultServerBurst,
	})
	if err != nil {
		return nil, err
	}

	// The client will log info the console that we don't really care about.
	cfg.WarningHandler = rest.NoWarnings{}

	// We only need to interact with UCP's store types.
	scheme := runtime.NewScheme()

	// Safe to ignore, this will only fail for duplicates, which there clearly won't be.
	_ = ucpv1alpha1.AddToScheme(scheme)

	options := runtimeclient.Options{
		Scheme: scheme,
	}

	rc, err := runtimeclient.New(cfg, options)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize APIServer client: %w", err)
	}

	client := apiserverstore.NewAPIServerClient(rc, opt.APIServer.Namespace)
	return client, nil
}

// initInMemoryClient creates a new in-memory store client.
func initInMemoryClient(ctx context.Context, opt Options) (store.Client, error) {
	return inmemory.NewClient(), nil
}

// envVarPattern matches ${VAR_NAME} references for expansion against environment variables.
var envVarPattern = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// expandEnvURL replaces ${VAR} references in the connection URL with the values of the
// corresponding environment variables. It returns an error if any referenced variable is unset,
// so a missing or mistyped variable fails fast with a clear message instead of silently producing
// a malformed connection string.
func expandEnvURL(url string) (string, error) {
	var missing []string
	expanded := envVarPattern.ReplaceAllStringFunc(url, func(match string) string {
		varName := envVarPattern.FindStringSubmatch(match)[1]
		val, ok := os.LookupEnv(varName)
		if !ok {
			missing = append(missing, varName)
		}
		return val
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("required environment variable(s) not set: %s", strings.Join(missing, ", "))
	}
	return expanded, nil
}

// initPostgreSQLClient creates a new PostgreSQL store client.
func initPostgreSQLClient(ctx context.Context, opt Options) (store.Client, error) {
	if opt.PostgreSQL.URL == "" {
		return nil, errors.New("failed to initialize PostgreSQL client: URL is required")
	}

	config, err := newPostgreSQLPoolConfig(opt.PostgreSQL, newAzureTokenCredential)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL client: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL client: %w", err)
	}

	return postgres.NewPostgresClient(pool), nil
}

// azureCredentialFactory creates the token credential for Microsoft Entra authentication.
type azureCredentialFactory func(PostgreSQLAzureOptions) (azcore.TokenCredential, error)

// newPostgreSQLPoolConfig parses the connection URL and configures authentication.
func newPostgreSQLPoolConfig(opt PostgreSQLOptions, newCredential azureCredentialFactory) (*pgxpool.Config, error) {
	url, err := expandEnvURL(opt.URL)
	if err != nil {
		return nil, err
	}

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}

	switch opt.AuthMode {
	case "", PostgreSQLAuthPassword:
		if opt.Azure != (PostgreSQLAzureOptions{}) {
			return nil, errors.New("azure options require authMode azure")
		}
		return config, nil
	case PostgreSQLAuthAzure:
	default:
		return nil, fmt.Errorf("unsupported authMode %q", opt.AuthMode)
	}

	if os.Getenv("PGPASSWORD") != "" {
		return nil, errors.New("authMode azure does not use a password: unset PGPASSWORD")
	}
	if urlHasPassword(url) {
		return nil, errors.New("authMode azure does not use a password: remove it from the URL")
	}
	if keywordHasPassword(url) {
		return nil, errors.New("authMode azure does not use a password: remove it from the connection string")
	}
	if config.ConnConfig.Password != "" {
		return nil, errors.New("authMode azure does not use a password: remove it from the connection string, the PostgreSQL service file (PGSERVICEFILE), or the password file (PGPASSFILE)")
	}
	if !verifiesServer(config.ConnConfig) {
		return nil, errors.New("authMode azure requires TLS with certificate and hostname verification (sslmode=verify-full) so the access token is only sent to a verified server")
	}

	credential, err := newCredential(opt.Azure)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credential: %w", err)
	}
	tokenTimeout := config.ConnConfig.ConnectTimeout
	if tokenTimeout <= 0 {
		tokenTimeout = defaultAzureTokenTimeout
	}
	config.BeforeConnect = azureTokenBeforeConnect(credential, tokenTimeout)
	return config, nil
}

// urlHasPassword reports whether a postgres:// URL sets a password, even an empty one, in the user
// information or as a query parameter. An empty password there can hide a password from another
// source in the parsed configuration. The URL is split by hand because net/url rejects the multi-host form that pgx accepts.
func urlHasPassword(connString string) bool {
	rest, ok := strings.CutPrefix(connString, "postgresql://")
	if !ok {
		if rest, ok = strings.CutPrefix(connString, "postgres://"); !ok {
			return false
		}
	}
	rest, _, _ = strings.Cut(rest, "#")
	authority := rest
	if i := strings.IndexAny(rest, "/?"); i >= 0 {
		authority = rest[:i]
	}
	if i := strings.LastIndex(authority, "@"); i >= 0 && strings.Contains(authority[:i], ":") {
		return true
	}
	_, query, _ := strings.Cut(rest, "?")
	values, _ := neturl.ParseQuery(query)
	return values.Has("password")
}

// keywordHasPassword reports whether a keyword/value connection string sets the password keyword,
// even to an empty value. It follows the libpq quoting and backslash-escaping rules that pgx uses, so
// text such as password= inside another keyword's quoted value is not mistaken for the keyword.
func keywordHasPassword(connString string) bool {
	if strings.HasPrefix(connString, "postgres://") || strings.HasPrefix(connString, "postgresql://") {
		return false
	}
	const space = " \t\n\r\v\f"
	s := strings.TrimLeft(connString, space)
	for s != "" {
		key, rest, ok := strings.Cut(s, "=")
		if !ok {
			return false
		}
		if strings.Trim(key, space) == "password" {
			return true
		}
		s = strings.TrimLeft(rest, space)
		end := 0
		if strings.HasPrefix(s, "'") {
			s = s[1:]
			for end < len(s) && s[end] != '\'' {
				if s[end] == '\\' {
					end++
				}
				end++
			}
			end++ // the closing quote
		} else {
			for end < len(s) && !strings.ContainsRune(space, rune(s[end])) {
				if s[end] == '\\' {
					end++
				}
				end++
			}
		}
		if end > len(s) {
			return false
		}
		s = strings.TrimLeft(s[end:], space)
	}
	return false
}

// verifiesServer reports whether every connection attempt, including fallbacks, uses TLS with
// certificate and hostname verification. Only sslmode=verify-full satisfies this.
func verifiesServer(config *pgx.ConnConfig) bool {
	verified := func(tlsConfig *tls.Config) bool {
		return tlsConfig != nil && !tlsConfig.InsecureSkipVerify
	}
	if !verified(config.TLSConfig) {
		return false
	}
	for _, fallback := range config.Fallbacks {
		if !verified(fallback.TLSConfig) {
			return false
		}
	}
	return true
}

// newAzureTokenCredential creates the Azure credential selected by the options.
func newAzureTokenCredential(opt PostgreSQLAzureOptions) (azcore.TokenCredential, error) {
	switch opt.Credential {
	case "", PostgreSQLAzureWorkloadIdentity:
		return azidentity.NewWorkloadIdentityCredential(&azidentity.WorkloadIdentityCredentialOptions{ClientID: opt.ClientID})
	case PostgreSQLAzureManagedIdentity:
		options := &azidentity.ManagedIdentityCredentialOptions{}
		if opt.ClientID != "" {
			options.ID = azidentity.ClientID(opt.ClientID)
		}
		return azidentity.NewManagedIdentityCredential(options)
	default:
		return nil, fmt.Errorf("unsupported azure credential %q", opt.Credential)
	}
}

// azurePostgreSQLScope is the Microsoft Entra scope for Azure Database for PostgreSQL in the public cloud.
const azurePostgreSQLScope = "https://ossrdbms-aad.database.windows.net/.default"

// defaultAzureTokenTimeout bounds token requests when the connection string does not set connect_timeout.
const defaultAzureTokenTimeout = 30 * time.Second

// azureTokenBeforeConnect returns a pgxpool hook that sets a Microsoft Entra access token as the
// password of each new connection. The server checks the token only at login, so open connections
// are unaffected when it expires; the credential caches tokens and renews them before expiry.
// The token request is bounded by timeout because it runs before pgx applies connect_timeout.
func azureTokenBeforeConnect(credential azcore.TokenCredential, timeout time.Duration) func(context.Context, *pgx.ConnConfig) error {
	return func(ctx context.Context, config *pgx.ConnConfig) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		token, err := credential.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{azurePostgreSQLScope}})
		if err != nil {
			return fmt.Errorf("failed to get Microsoft Entra token for PostgreSQL: %w", err)
		}
		config.Password = token.Token
		return nil
	}
}
