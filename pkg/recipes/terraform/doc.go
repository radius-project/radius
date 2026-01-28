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

/*
Package terraform provides the Terraform recipe driver and executor for Radius.

# Terraform Binary Lookup

When a recipe executes Terraform, the binary is located using the following priority order:

 1. Recipe execution calls Install() which delegates to ensureGlobalTerraformBinary()

 2. The function first checks for /terraform/current symlink, which is created by
    the Terraform installer API (rad terraform install command)

 3. If found, the symlink is resolved to /terraform/versions/{version}/terraform
    and the binary is verified by running "terraform version"

 4. If the installer binary is working, it is used directly - no download needed

 5. If not found or not working, falls back to the global shared binary at
    /terraform/.terraform-global/terraform

 6. If the global binary doesn't exist, downloads Terraform via hc-install library

# Path Summary

  - Installer API path: /terraform/current -> /terraform/versions/{version}/terraform
  - Global shared path: /terraform/.terraform-global/terraform
  - Global marker file: /terraform/.terraform-global/.terraform-ready

# Environment Variables (Testing)

  - TERRAFORM_TEST_GLOBAL_DIR: Override the global terraform directory for testing
  - TERRAFORM_TEST_INSTALLER_DIR: Override the installer API directory for testing
*/
package terraform
