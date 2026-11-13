{{/*
Only completed schema4 journals may participate in ordinary Helm rendering.
An active journal is never interpreted as permission to switch an issuer or
overwrite overlap. The CLI must validate DER sets and observation barriers.
*/}}
{{- define "radius.lifecycleSnapshot" -}}
{{- $namespace := lookup "v1" "Namespace" "" .Release.Namespace -}}
{{- if dig "metadata" "annotations" "radius.io/aggregation-bootstrap-intent" "" $namespace -}}
{{- fail "Legacy bootstrap intent without its journal migration cannot be reinterpreted as token/CSI readiness." -}}
{{- end -}}
{{- if lookup "v1" "ConfigMap" .Release.Namespace "radius-aggregation-bootstrap" -}}
{{- fail "Legacy aggregation bootstrap journal requires explicit recovery with the matching pre-token release." -}}
{{- end -}}
{{- $journal := lookup "v1" "ConfigMap" .Release.Namespace "radius-ca-rotation" -}}
{{- $result := dict -}}
{{- if $journal -}}
{{- if or (ne (dig "metadata" "labels" "radius.io/trust-owner" "" $journal) .Release.Name)
  (dig "metadata" "ownerReferences" (list) $journal)
  (dig "metadata" "deletionTimestamp" "" $journal)
  (dig "metadata" "annotations" "meta.helm.sh/release-name" "" $journal)
  (empty $journal.metadata.uid) (empty $journal.metadata.resourceVersion)
  (ne (len $journal.data) 1) (not (empty $journal.binaryData)) -}}
{{- fail "CA lifecycle journal has foreign or incomplete ownership." -}}
{{- end -}}
{{- $encoded := index $journal.data "state.json" | default "" -}}
{{- if or (empty $encoded) (gt (len $encoded) 65536) -}}{{- fail "CA lifecycle journal is empty or too large." -}}{{- end -}}
{{- $state := mustFromJson $encoded -}}
{{- if or (ne (toString $state.schemaVersion) "4") (ne $state.namespace .Release.Namespace) (ne $state.release .Release.Name) -}}
{{- fail "Legacy or foreign CA journal cannot be reinterpreted as token/CSI evidence." -}}
{{- end -}}
{{- if ne $state.phase "complete" -}}{{- fail "CA lifecycle is unfinished; Helm must preserve its staged overlap without modifying workloads or issuers." -}}{{- end -}}
{{- if or (ne $state.mode (include "radius.authz.certificates.mode" .)) (ne $state.provider (include "radius.authz.tls.provider" .)) -}}
{{- fail "CA lifecycle mode/provider does not match the selected chart settings." -}}
{{- end -}}
{{- if not (regexMatch `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$` ($state.id | default "")) -}}
{{- fail "CA lifecycle transaction identity is invalid." -}}
{{- end -}}
{{- $planes := list "aggregation" -}}
{{- if eq $state.provider "radius" -}}{{- $planes = append $planes "internal" -}}{{- end -}}
{{- if ne (len $state.planes) (len $planes) -}}{{- fail "CA lifecycle must preserve every enabled trust plane." -}}{{- end -}}
{{- range $plane := $planes -}}
{{- $record := index $state.planes $plane | default dict -}}
{{- $pin := $record.source | default dict -}}
{{- $kind := "ConfigMap" -}}{{- $name := "radius-mtls-ca" -}}
{{- if eq $plane "aggregation" -}}{{- $kind = "Secret" -}}{{- $name = "radius-apiserver-trust" -}}{{- end -}}
{{- if or (ne $pin.kind $kind) (ne $pin.name $name) (empty $pin.uid) (empty $pin.resourceVersion) (empty $record.newCA) -}}
{{- fail "CA lifecycle public source pin is incomplete." -}}
{{- end -}}
{{- $source := lookup "v1" $kind $.Release.Namespace $name -}}
{{- if not $source -}}{{- fail "CA lifecycle public source is missing." -}}{{- end -}}
{{- $annotations := $source.metadata.annotations | default dict -}}
{{- $revision := printf "%s/complete" $state.id -}}
{{- if or (ne $source.metadata.uid $pin.uid) (empty $source.metadata.resourceVersion)
  (dig "metadata" "deletionTimestamp" "" $source) (dig "metadata" "ownerReferences" (list) $source)
  (ne (dig "metadata" "labels" "app.kubernetes.io/managed-by" "" $source) "Helm")
  (ne (index $annotations "meta.helm.sh/release-name") $.Release.Name)
  (ne (index $annotations "meta.helm.sh/release-namespace") $.Release.Namespace)
  (ne (index $annotations "radius.io/trust-journal-uid") $journal.metadata.uid)
  (ne (index $annotations "radius.io/trust-revision") $revision)
  (ne (len $source.data) 1) (not (empty $source.binaryData)) (not (empty $source.stringData)) -}}
{{- fail "CA lifecycle public source identity, ownership or contents changed." -}}
{{- end -}}
{{- $bundle := index $source.data "ca.crt" | default "" -}}
{{- if eq $kind "Secret" -}}
{{- if or (ne $source.type "Opaque") (ne (index $annotations "cert-manager.io/allow-direct-injection") "true") -}}
{{- fail "Aggregation lifecycle source must remain public-only and injection-enabled." -}}
{{- end -}}
{{- $bundle = b64dec $bundle -}}
{{- end -}}
{{- if or (empty $bundle) (ne $bundle (b64dec $pin.bundle)) (ne (sha256sum $bundle) $pin.sha256) -}}
{{- fail "CA lifecycle source differs from its exact completed snapshot." -}}
{{- end -}}
{{- if ne $bundle (b64dec $record.newCA) -}}
{{- fail "Completed CA lifecycle source must contain exactly the target anchor, not retained overlap." -}}
{{- end -}}
{{- $issuer := dict "group" $record.issuer.Group "kind" $record.issuer.Kind "name" $record.issuer.Name -}}
{{- if or (ne $issuer.group "cert-manager.io") (not (has $issuer.kind (list "Issuer" "ClusterIssuer"))) (empty $issuer.name) -}}
{{- fail "CA lifecycle issuer reference is invalid." -}}
{{- end -}}
{{- $before := $record.previous | default dict -}}
{{- $target := $record.targetIssuer | default dict -}}
{{- if or (empty $before) (empty $target.uid) (empty $target.resourceVersion)
  (ne (toJson $target.reference) (toJson $record.issuer))
  (ne $before.source.kind $pin.kind) (ne $before.source.name $pin.name) (ne $before.source.uid $pin.uid)
  (empty $before.source.resourceVersion) (empty $before.source.bundle)
  (ne (sha256sum (b64dec $before.source.bundle)) $before.source.sha256)
  (not (has $before.mode (list "selfSigned" "caSecret" "issuerRef"))) -}}
{{- fail "CA lifecycle requires complete previous and target configuration snapshots." -}}
{{- end -}}
{{- if and $record.affected (eq $state.mode "issuerRef") (eq (toJson $before.issuer.reference) (toJson $target.reference)) -}}
{{- fail "IssuerRef replacement requires a distinct staged issuer, not same-name out-of-band replacement." -}}
{{- end -}}
{{- $issuerNamespace := $.Release.Namespace -}}
{{- if eq $issuer.kind "ClusterIssuer" -}}{{- $issuerNamespace = "" -}}{{- end -}}
{{- $liveIssuer := lookup "cert-manager.io/v1" $issuer.kind $issuerNamespace $issuer.name -}}
{{- if or (not $liveIssuer) (ne $target.namespace $issuerNamespace)
  (ne (dig "metadata" "uid" "" $liveIssuer) $target.uid)
  (empty (dig "metadata" "resourceVersion" "" $liveIssuer))
  (dig "metadata" "deletionTimestamp" "" $liveIssuer)
  (empty $liveIssuer.spec) (ne (sha256sum (toJson $liveIssuer.spec)) $target.specSHA256) -}}
{{- fail "CA lifecycle target issuer identity or configuration changed." -}}
{{- end -}}
{{- if and (ne $state.mode "issuerRef")
  (or (empty $record.signer) (ne $target.signingSecret $record.signer.name)
      (ne (dig "spec" "ca" "secretName" "" $liveIssuer) $record.signer.name)) -}}
{{- fail "CA lifecycle target issuer no longer selects its pinned signing Secret." -}}
{{- end -}}
{{- $expected := dict "group" "cert-manager.io" "kind" "Issuer" "name" "radius-ca-issuer" -}}
{{- if eq $plane "aggregation" -}}{{- $_ := set $expected "name" "radius-apiserver-issuer" -}}{{- end -}}
{{- if eq $state.mode "issuerRef" -}}{{- $expected = $.Values.global.rbac.certificates.issuerRef -}}{{- end -}}
{{- if ne (toJson $issuer) (toJson $expected) -}}
{{- fail "The selected chart cannot preserve this staged issuer; keep the lifecycle blocked rather than silently reverting issuance." -}}
{{- end -}}
{{- $_ := set $result $plane (dict "kind" $kind "name" $name "key" "ca.crt" "bundle" $bundle
  "uid" $pin.uid "resourceVersion" $source.metadata.resourceVersion "contentDigest" (sha256sum $bundle)
  "issuer" $issuer "issuerDigest" (sha256sum (toJson $issuer))
  "digest" (sha256sum (printf "%s\n%s" $bundle (toJson $issuer)))
  "journalUID" $journal.metadata.uid "revision" $revision) -}}
{{- end -}}
{{- end -}}
{{- toJson $result -}}
{{- end -}}
