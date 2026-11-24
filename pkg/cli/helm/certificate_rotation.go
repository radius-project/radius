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
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	rotationJournalName = "radius-ca-rotation"
	rotationJournalKey  = "state.json"
	rotationOwner       = "radius.io/trust-owner"
	rotationRevision    = "radius.io/trust-revision"
	rotationJournalUID  = "radius.io/trust-journal-uid"
	radiusAPIService    = "v1alpha3.api.ucp.dev"
	maxRotationBytes    = 64 * 1024
)

var apiServiceResource = schema.GroupVersionResource{
	Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices",
}

var (
	rotationCertificateResource = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}
	rotationIssuerResource      = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"}
)

type rotationScope string

const (
	rotationComponentsAndAggregation rotationScope = "componentsAndAggregation"
	rotationAggregationOnly          rotationScope = "aggregationOnly"
)

type rotationPhase string

const (
	rotationPublish  rotationPhase = "publish"
	rotationAdopt    rotationPhase = "adopt"
	rotationIssue    rotationPhase = "issue"
	rotationReady    rotationPhase = "ready"
	rotationRetire   rotationPhase = "retire"
	rotationComplete rotationPhase = "complete"
)

var rotationPhases = []rotationPhase{rotationPublish, rotationAdopt, rotationIssue, rotationReady, rotationRetire, rotationComplete}

// rotationStore owns only the public journal and APIService trust. It never writes
// a signing Secret, installs admission policy, or takes control of an injector.
type rotationStore struct {
	kube      kubernetes.Interface
	dynamic   dynamic.Interface
	namespace string
	release   string
	now       func() time.Time
}

func rotationOwned(object metav1.Object, namespace, release string) bool {
	return object.GetUID() != "" && object.GetResourceVersion() != "" && object.GetDeletionTimestamp() == nil &&
		object.GetAnnotations()["meta.helm.sh/release-name"] == release &&
		object.GetAnnotations()["meta.helm.sh/release-namespace"] == namespace
}

func checkRotationJSON(decoder *json.Decoder, token json.Token, depth int) error {
	if depth > 16 || token == nil {
		return errors.New("invalid or null CA rotation journal field")
	}
	delim, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	fields := map[string]bool{}
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return errors.New("invalid CA rotation journal")
			}
			field, ok := key.(string)
			if !ok || fields[field] {
				return errors.New("duplicate CA rotation journal field")
			}
			fields[field] = true
		}
		value, err := decoder.Token()
		if err != nil {
			return errors.New("invalid CA rotation journal")
		}
		if err := checkRotationJSON(decoder, value, depth+1); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("invalid CA rotation journal")
	}
	return nil
}

func apiServiceBundle(api *unstructured.Unstructured) ([]byte, error) {
	encoded, exists, err := unstructured.NestedString(api.Object, "spec", "caBundle")
	if err != nil || !exists {
		return nil, errors.New("Radius APIService has no public CA bundle")
	}
	bundle, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(bundle) == 0 || len(bundle) > maxRotationBytes {
		return nil, errors.New("Radius APIService has an invalid public CA bundle")
	}
	return bundle, nil
}
