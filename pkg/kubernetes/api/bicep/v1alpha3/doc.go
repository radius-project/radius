// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package v1alpha3

/*
Package bicep v1alpha3 contains the types and webhook definitions
for bicep types deployed to kubernetes.

These types are put into a separate group (bicep.dev) from the radius
types, as eventually there will be overlap with the ARM team's effort
to open source the deployment engine. Therefore, this piece could be
considered a separate piece from radius.

The long term goal is to define a deployment engine to orchastrate ordering
of radius types (and eventually all bicep types).

Validating webhooks today make sure the arm template sent in the
DeploymentTemplate CRD is valid.
*/
