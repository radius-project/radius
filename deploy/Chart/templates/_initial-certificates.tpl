{{/*
Only the lifecycle publishes initial public sources. A full Helm apply cannot
skip that stage or change a pending transaction. Completed journals are history,
not an override of a subsequent CA lifecycle.
*/}}
{{- define "radius.initialCertificateGuard" -}}
{{- $journal := lookup "v1" "ConfigMap" .Release.Namespace "radius-certificate-bootstrap" -}}
{{- if $journal -}}
{{- if or (empty $journal.metadata.uid) (empty $journal.metadata.resourceVersion)
    $journal.metadata.deletionTimestamp (not (empty $journal.metadata.ownerReferences))
    (ne (index ($journal.metadata.labels | default dict) "radius.io/trust-owner" | default "") .Release.Name)
    (ne (len ($journal.data | default dict)) 1) (not (empty $journal.binaryData)) -}}
{{- fail "Initial certificate journal has invalid ownership or identity." -}}
{{- end -}}
{{- $state := index $journal.data "state.json" | mustFromJson -}}
{{- $fields := list "version" "id" "namespace" "namespaceUID" "release" "mode" "provider" "phase" "chartSHA256" "intentSHA256" "valuesSHA256" "roots" "anchors" "sources" "issuers" -}}
{{- if ne (len $state) (len $fields) -}}{{- fail "Initial certificate journal has an unsupported field set." -}}{{- end -}}
{{- range $key, $_ := $state -}}
{{- if not (has $key $fields) -}}{{- fail "Initial certificate journal has an unknown field." -}}{{- end -}}
{{- end -}}
{{- if or (ne (int $state.version) 1) (empty $state.id)
    (ne $state.namespace .Release.Namespace) (ne $state.release .Release.Name)
    (not (has $state.mode (list "selfSigned" "caSecret" "issuerRef")))
    (not (has $state.provider (list "radius" "external")))
    (not (has $state.phase (list "preparing" "published" "complete"))) -}}
{{- fail "Initial certificate journal has an unsupported schema or identity." -}}
{{- end -}}
{{- $namespace := lookup "v1" "Namespace" "" .Release.Namespace -}}
{{- if or (empty $namespace) (ne $namespace.metadata.uid $state.namespaceUID) -}}
{{- fail "Initial certificate namespace identity changed." -}}
{{- end -}}
{{- if ne $state.phase "complete" -}}
{{- if ne $state.phase "published" -}}
{{- fail "Finish initial certificate public-source publication before applying the full chart." -}}
{{- end -}}
{{- if or (eq (include "radius.authz.mode" .) "off")
    (ne (include "radius.authz.certificates.mode" .) $state.mode)
    (ne (include "radius.authz.tls.provider" .) $state.provider)
    (ne (sha256sum (toJson .Values)) $state.valuesSHA256) -}}
{{- fail "Initial certificate retry changed its recorded values or provider." -}}
{{- end -}}
{{- $planes := list "aggregation" -}}
{{- if eq $state.provider "radius" -}}{{- $planes = append $planes "internal" -}}{{- end -}}
{{- if or (ne (len $state.sources) (len $planes)) (ne (len $state.anchors) (len $planes)) -}}
{{- fail "Initial certificate publication is incomplete." -}}
{{- end -}}
{{- range $plane := $planes -}}
{{- $pin := index $state.sources $plane -}}
{{- $kind := "Secret" -}}{{- $name := "radius-apiserver-trust" -}}
{{- if eq $plane "internal" -}}{{- $kind = "ConfigMap" -}}{{- $name = "radius-mtls-ca" -}}{{- end -}}
{{- $source := lookup "v1" $kind $.Release.Namespace $name -}}
{{- $annotations := dig "metadata" "annotations" (dict) $source -}}
{{- $actual := index ($source.data | default dict) "ca.crt" | default "" -}}
{{- if eq $kind "Secret" -}}{{- $actual = b64dec $actual -}}{{- end -}}
{{- if or (empty $source) (ne $pin.name $name) (ne $pin.kind $kind)
    (empty $pin.uid) (ne $source.metadata.uid $pin.uid)
    $source.metadata.deletionTimestamp (not (empty $source.metadata.ownerReferences))
    (ne (index $annotations "radius.io/certificate-bootstrap" | default "") $journal.metadata.uid)
    (ne (index $annotations "radius.io/certificate-mode" | default "") $state.mode)
    (ne (index $annotations "meta.helm.sh/release-name" | default "") $.Release.Name)
    (ne (index $annotations "meta.helm.sh/release-namespace" | default "") $.Release.Namespace)
    (ne (index ($source.metadata.labels | default dict) "app.kubernetes.io/managed-by" | default "") "Helm")
    (ne (index $annotations "helm.sh/resource-policy" | default "") "keep")
    (ne (len ($source.data | default dict)) 1) (not (empty $source.binaryData))
    (empty $actual) (ne $actual (b64dec $pin.bundle))
    (ne (sha256sum $actual) $pin.sha256)
    (ne $actual (b64dec (index $state.anchors $plane).ca)) -}}
{{- fail "Initial certificate public-source identity or exact contents changed." -}}
{{- end -}}
{{- if and (eq $kind "Secret") (or (ne $source.type "Opaque") (ne (index $annotations "cert-manager.io/allow-direct-injection" | default "") "true")) -}}
{{- fail "Initial aggregation source must remain a public-only injectable Secret." -}}
{{- end -}}
{{- end -}}
{{- range $key, $pin := $state.roots -}}
{{- $parts := splitList "/" $key -}}
{{- if or (ne (len $parts) 3) (ne (first $parts) "cert-manager.io") -}}{{- fail "Invalid initial root reference." -}}{{- end -}}
{{- $kind := "Certificate" -}}
{{- if eq (index $parts 1) "issuers" -}}{{- $kind = "Issuer" -}}{{- end -}}
{{- $object := lookup "cert-manager.io/v1" $kind $.Release.Namespace (index $parts 2) -}}
{{- if or (empty $object) (ne $object.metadata.uid $pin.uid)
    (ne (sha256sum (toJson $object.spec)) $pin.specSHA256)
    (ne (index ($object.metadata.annotations | default dict) "radius.io/certificate-bootstrap" | default "") $journal.metadata.uid) -}}
{{- fail "Initial root identity or configuration changed." -}}
{{- end -}}
{{- end -}}
{{- range $_, $pin := $state.issuers -}}
{{- $object := lookup "cert-manager.io/v1" $pin.reference.kind $pin.namespace $pin.reference.name -}}
{{- if or (empty $object) (ne $object.metadata.uid $pin.uid) (ne $object.metadata.resourceVersion $pin.resourceVersion)
    (ne (sha256sum (toJson $object.spec)) $pin.specSHA256) -}}
{{- fail "Initial external issuer identity or configuration changed." -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
