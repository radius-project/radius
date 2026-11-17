/*
Copyright 2026 The Radius Authors.

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

package helm

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ucpMutationGuard func(context.Context, string, metav1.PartialObjectMetadata) (retain bool, err error)
type ucpMutationGuardKey struct{}

type ucpMutationGuardError struct{ err error }

func (e *ucpMutationGuardError) Error() string { return e.err.Error() }
func (e *ucpMutationGuardError) Unwrap() error { return e.err }

func withUCPMutationGuard(ctx context.Context, guard ucpMutationGuard) context.Context {
	if guard == nil {
		return ctx
	}
	previous, _ := ctx.Value(ucpMutationGuardKey{}).(ucpMutationGuard)
	return context.WithValue(ctx, ucpMutationGuardKey{}, ucpMutationGuard(func(ctx context.Context, verb string, object metav1.PartialObjectMetadata) (bool, error) {
		retain := false
		if previous != nil {
			var err error
			retain, err = previous(ctx, verb, object)
			if err != nil {
				return false, err
			}
		}
		keep, err := guard(ctx, verb, object)
		return retain || keep, err
	}))
}

// checkUCPMutation supplies metadata only. A retention decision is valid solely
// for deletion; it must never turn a rejected update into apparent success.
func checkUCPMutation(ctx context.Context, verb, kind string, object metav1.Object) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	guard, _ := ctx.Value(ucpMutationGuardKey{}).(ucpMutationGuard)
	if guard == nil {
		return false, nil
	}
	target := metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{Kind: kind},
		ObjectMeta: metav1.ObjectMeta{
			Name: object.GetName(), Namespace: object.GetNamespace(),
			UID: object.GetUID(), ResourceVersion: object.GetResourceVersion(),
			Labels: object.GetLabels(), Annotations: object.GetAnnotations(),
			OwnerReferences: object.GetOwnerReferences(), DeletionTimestamp: object.GetDeletionTimestamp(),
		},
	}
	retain, err := guard(ctx, verb, *target.DeepCopy())
	if err != nil {
		return false, &ucpMutationGuardError{err: err}
	}
	if retain && verb != "delete" {
		return false, errors.New("UCP mutation retention applies only to deletion")
	}
	return retain, ctx.Err()
}
