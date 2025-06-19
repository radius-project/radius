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

// Package customsource provides a custom implementation for downloading Terraform
// binaries from private registries with custom TLS configuration.
//
// # Why Custom Source?
//
// The standard hc-install library has limitations when working with private registries:
//   - No Custom TLS Configuration: Cannot use custom CA certificates or skip TLS verification
//   - No Authentication Headers: Cannot add bearer tokens or other auth headers
//   - No HTTP Client Customization: Cannot configure timeouts, proxies, or other HTTP settings
//
// These limitations make it difficult to use hc-install in enterprise environments with:
//   - Self-signed certificates
//   - Private certificate authorities
//   - Air-gapped networks
//   - Authenticated private registries
//
// # Implementation Note
//
// This package provides a standalone implementation that doesn't integrate directly with
// hc-install's installer.Ensure() method due to the library's use of internal packages.
// Instead, it provides similar functionality that can be used as an alternative when
// custom TLS configuration is needed.
//
// # Features
//
// The CustomRegistrySource supports:
//   - Custom CA certificates for private registries with self-signed certificates
//   - Skip TLS verification for development/testing environments (not recommended for production)
//   - Bearer token authentication for secured registries
//   - Configurable HTTP timeouts
//   - SHA256 checksum verification
//   - Compatible registry structure with HashiCorp releases
//
// # Registry Structure
//
// Your private registry must mirror the HashiCorp releases structure:
//
//	https://your-registry.com/
//	├── terraform/
//	│   ├── index.json                    # Version index
//	│   ├── 1.5.0/
//	│   │   ├── terraform_1.5.0_darwin_amd64.zip
//	│   │   ├── terraform_1.5.0_linux_amd64.zip
//	│   │   ├── terraform_1.5.0_windows_amd64.zip
//	│   │   └── terraform_1.5.0_SHA256SUMS
//	│   └── 1.5.1/
//	│       └── ...
//
// The index.json file must contain version metadata in the following format:
//
//	{
//	  "versions": {
//	    "1.5.0": {
//	      "version": "1.5.0",
//	      "builds": [
//	        {
//	          "os": "darwin",
//	          "arch": "amd64",
//	          "filename": "terraform_1.5.0_darwin_amd64.zip",
//	          "url": "terraform/1.5.0/terraform_1.5.0_darwin_amd64.zip"
//	        }
//	      ],
//	      "shasums": "terraform_1.5.0_SHA256SUMS"
//	    }
//	  }
//	}
//
// # Usage Example
//
//	import (
//	    "context"
//	    "github.com/hashicorp/go-version"
//	    "github.com/hashicorp/hc-install/product"
//	    "github.com/radius-project/radius/pkg/recipes/terraform/customsource"
//	)
//
//	func installTerraform() error {
//	    ctx := context.Background()
//
//	    // Create custom source
//	    source := &customsource.CustomRegistrySource{
//	        Product:            product.Terraform,
//	        Version:            version.Must(version.NewVersion("1.5.0")),
//	        BaseURL:            "https://private-registry.example.com",
//	        InstallDir:         "/opt/terraform",
//	        AuthToken:          os.Getenv("REGISTRY_TOKEN"),
//	        CACertPEM:          loadCABundle(),
//	        InsecureSkipVerify: false,
//	        Timeout:            300, // 5 minutes
//	    }
//
//	    // Install directly (not through hc-install installer)
//	    execPath, err := source.Install(ctx)
//	    if err != nil {
//	        return err
//	    }
//
//	    fmt.Printf("Terraform installed at: %s\n", execPath)
//	    return nil
//	}
//
// # Security Considerations
//
//   - Always Use HTTPS: The implementation validates that URLs use HTTPS unless explicitly overridden
//   - Avoid Skip Verify: Only use InsecureSkipVerify in development/testing environments
//   - Protect Auth Tokens: Store authentication tokens securely (e.g., environment variables, secret stores)
//   - Verify Checksums: The implementation automatically verifies SHA256 checksums when available
//
// # Limitations
//
//   - Latest Version: Currently requires specifying an exact version
//   - Signature Verification: GPG signature verification is not yet implemented
//   - Mirror Structure: Must follow HashiCorp's directory structure exactly
//   - No hc-install Integration: Cannot be used with installer.Ensure() due to internal package usage
package customsource
