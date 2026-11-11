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
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// legacyMTLSIssuerName is the Issuer that earlier authorization charts used for per-component client certificates.
// The current chart no longer creates it, and cert-manager's default approver no longer approves it.
const legacyMTLSIssuerName = "radius-mtls-issuer"

// unapprovedIssuerNames are the namespaced Issuers whose Certificate requests cert-manager's default approver does not
// approve after rad limits it: radius-ca-issuer, which only Radius's csi-driver-spiffe approver may approve, and the
// legacy client certificate issuer.
var unapprovedIssuerNames = []string{RadiusCAIssuerName, legacyMTLSIssuerName}

// unapprovedIssuerCertificates returns the namespace/name of each cert-manager Certificate in manifest that requests
// its certificate from one of unapprovedIssuerNames. Radius charts before csi-driver-spiffe issued component client
// certificates from radius-ca-issuer, and later charts issued them from radius-mtls-issuer; current charts request
// from radius-ca-issuer only through csi-driver-spiffe.
func unapprovedIssuerCertificates(manifest string) ([]string, error) {
	var certificates []string
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	for {
		var document map[string]any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return certificates, nil
		}
		if err != nil {
			return nil, err
		}
		if document == nil {
			continue
		}

		object := unstructured.Unstructured{Object: document}
		if object.GetKind() != "Certificate" || object.GroupVersionKind().Group != "cert-manager.io" {
			continue
		}
		name, _, _ := unstructured.NestedString(object.Object, "spec", "issuerRef", "name")
		kind, _, _ := unstructured.NestedString(object.Object, "spec", "issuerRef", "kind")
		group, _, _ := unstructured.NestedString(object.Object, "spec", "issuerRef", "group")
		if !slices.Contains(unapprovedIssuerNames, name) || (kind != "" && kind != "Issuer") || (group != "" && group != "cert-manager.io") {
			continue
		}
		certificates = append(certificates, fmt.Sprintf("%s/%s (Issuer %s)", object.GetNamespace(), object.GetName(), name))
	}
}

// checkRollbackCertificateIssuers refuses a rollback to a revision that issues Certificates from an Issuer in
// unapprovedIssuerNames. Rollback keeps cert-manager's default approver limited to the signers of the current rad, so
// those Certificates would stay unapproved: they could not be issued or renewed, and the rolled-back components could
// not authenticate to each other once their certificates expired.
func checkRollbackCertificateIssuers(history []*releasev1.Release, revision int) error {
	for _, rel := range history {
		if rel == nil || rel.Version != revision {
			continue
		}
		certificates, err := unapprovedIssuerCertificates(rel.Manifest)
		if err != nil {
			return fmt.Errorf("failed to read the manifest of Radius revision %d: %w", revision, err)
		}
		if len(certificates) == 0 {
			return nil
		}
		return clierrors.Message("Radius cannot roll back to revision %d because it issues these component client Certificates: %s. "+
			"rad limits cert-manager's default approver so that it does not approve requests for these Issuers "+
			"(only Radius's csi-driver-spiffe approver may approve %q), so cert-manager would never issue or renew these certificates. "+
			"Roll back to a revision that does not create them, or keep the current version. "+
			"Run 'rad rollback kubernetes --list-revisions' to see the revisions.",
			revision, strings.Join(certificates, ", "), RadiusCAIssuerName)
	}
	return nil
}
