// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// processors contains the shared logic and interfaces for implementing a resource processor.
//
// Resource processors are responsible for processing the results of recipe execution or any
// other change to the lifecycle of a link resource.
//
// For example a mongo processor might take the results of executing a recipe and compute
// and store the connection string as part of the resource data model.
package processors
