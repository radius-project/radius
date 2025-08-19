package terraform

// TerraformConfig contains the configuration for Terraform integration.
type TerraformConfig struct {
	// Authentication contains the authentication information used to access private Terraform module sources.
	Authentication AuthConfig `json:"authentication,omitempty"`
	// Providers contains the configuration for Terraform Recipe Providers.
	Providers map[string][]ProviderConfig `json:"providers,omitempty"`
	// ProviderMirror contains the configuration for a Terraform provider mirror.
	ProviderMirror *TerraformProviderMirrorConfig `json:"providerMirror,omitempty"`
	// ModuleRegistries contains the configuration for Terraform module registries.
	ModuleRegistries map[string]TerraformModuleRegistryConfig `json:"moduleRegistries,omitempty"`
	// Version contains the configuration for the Terraform binary installation.
	Version *TerraformVersionConfig `json:"version,omitempty"`
}

// TerraformModuleRegistryConfig contains the configuration for a Terraform module registry.
type TerraformModuleRegistryConfig struct {
	// URL is the base URL of the module registry.
	URL string `json:"url"`
	// Authentication is the authentication configuration for accessing the module registry.
	Authentication *RegistryAuthConfig `json:"authentication,omitempty"`
	// TLS is the TLS configuration for connecting to the module registry.
	TLS *TLSConfig `json:"tls,omitempty"`
}

// TerraformProviderMirrorConfig contains the configuration for a Terraform provider mirror.
type TerraformProviderMirrorConfig struct {
	// URL is the base URL of the provider mirror.
	URL string `json:"url"`
	// Type is the type of the provider mirror (e.g., "filesystem", "network").
	Type string `json:"type"`
	// Authentication is the authentication configuration for accessing the module registry.
	Authentication *RegistryAuthConfig `json:"authentication,omitempty"`
	// TLS is the TLS configuration for connecting to the provider mirror.
	TLS *TLSConfig `json:"tls,omitempty"`
}

// AuthConfig contains the authentication information.
type AuthConfig struct {
	// Token is the authentication token.
	Token string `json:"token,omitempty"`
	// Username is the username for basic authentication.
	Username string `json:"username,omitempty"`
	// Password is the password for basic authentication.
	Password string `json:"password,omitempty"`
}

// ProviderConfig contains the configuration for a Terraform provider.
type ProviderConfig struct {
	// Name is the name of the provider.
	Name string `json:"name"`
	// Source is the source address of the provider.
	Source string `json:"source"`
	// Version is the version constraint for the provider.
	Version string `json:"version,omitempty"`
}

// TLSConfig contains the TLS configuration.
type TLSConfig struct {
	// CAFile is the path to the CA certificate file.
	CAFile string `json:"caFile,omitempty"`
	// CertFile is the path to the client certificate file.
	CertFile string `json:"certFile,omitempty"`
	// KeyFile is the path to the client private key file.
	KeyFile string `json:"keyFile,omitempty"`
}

// RegistryAuthConfig contains the authentication configuration for a registry.
type RegistryAuthConfig struct {
	// Token is the authentication token.
	Token string `json:"token,omitempty"`
	// Username is the username for basic authentication.
	Username string `json:"username,omitempty"`
	// Password is the password for basic authentication.
	Password string `json:"password,omitempty"`
}

// TerraformVersionConfig contains the configuration for the Terraform binary installation.
type TerraformVersionConfig struct {
	// RequiredVersion is the required Terraform version.
	RequiredVersion string `json:"requiredVersion,omitempty"`
	// InstalledVersion is the installed Terraform version.
	InstalledVersion string `json:"installedVersion,omitempty"`
}
