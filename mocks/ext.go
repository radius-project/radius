// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mocks

// Mocks for external APIs go here.

//go:generate mockgen -destination=./mock_k8s.go -package=mocks sigs.k8s.io/controller-runtime/pkg/client Client
