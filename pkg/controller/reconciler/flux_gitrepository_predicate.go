/*
Copyright 2024 The Radius Authors.
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

package reconciler

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
)

// Based on: https://github.com/fluxcd/source-watcher/blob/main/controllers/gitrepository_predicate.go

// GitRepositoryRevisionChangePredicate triggers an update event
// when a GitRepository revision changes.
type GitRepositoryRevisionChangePredicate struct {
	predicate.Funcs
}

func (*GitRepositoryRevisionChangePredicate) Create(e event.CreateEvent) bool {
	if e.Object == nil {
		return false
	}

	src, ok := e.Object.(sourcev1.Source)
	if !ok || src == nil || src.GetArtifact() == nil {
		return false
	}

	return true
}

func (*GitRepositoryRevisionChangePredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	oldSource, ok := e.ObjectOld.(sourcev1.Source)
	if !ok {
		return false
	}

	newSource, ok := e.ObjectNew.(sourcev1.Source)
	if !ok {
		return false
	}

	if oldSource.GetArtifact() == nil && newSource.GetArtifact() != nil {
		return true
	}

	if oldSource.GetArtifact() != nil && newSource.GetArtifact() != nil &&
		oldSource.GetArtifact().Revision != newSource.GetArtifact().Revision {
		return true
	}

	return false
}
