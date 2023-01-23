// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// sdk defines the functionality for interacting with Radius as a client. This includes
// opening a connection to the Radius control plane for making API calls as well as client
// APIs.
//
// The sdk package can be safely used from the CLI and RPs in Radius as well as external
// packages. This means that the sdk package CANNOT reference other packages in the
// Radius codebase as it will be used from basically everywhere.
package sdk
