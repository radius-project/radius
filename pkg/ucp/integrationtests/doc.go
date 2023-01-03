// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// integrationtests contains integration tests that run UCP using an in-memory server and in-memory data stores.
//
// Many of UCP's features are stateful in a global way (planes, credentials, etc) so testing in-memory is faster and more convenient
// than testing a deployed instance of UCP.
//
// We also know that UCP's use-cases are different from that of a traditional RP, therefore we want to pay special attention to the details.
package integrationtests
