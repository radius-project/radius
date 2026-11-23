{{/*
Steady-state certificate contract. A10 replaces the journal guard with its validated
snapshot resolver; an ordinary Helm render must never overwrite staged overlap.
*/}}
{{- define "radius.authz.certificates.mode" -}}
{{- $c := .Values.global.rbac.certificates | default dict -}}
{{- $mode := "selfSigned" -}}
{{- if hasKey $c "mode" -}}{{- $mode = toString $c.mode -}}{{- end -}}
{{- if not (has $mode (list "selfSigned" "caSecret" "issuerRef")) -}}
{{- fail (printf "global.rbac.certificates.mode=%q is not valid. Use selfSigned, caSecret or issuerRef." $mode) -}}
{{- end -}}
{{- $mode -}}
{{- end -}}

{{- define "radius.authz.certificates.issuer" -}}
{{- $lifecycle := include "radius.lifecycleSnapshot" .root | mustFromJson -}}
{{- $source := index $lifecycle .plane -}}
{{- if and $lifecycle (not $source) (not (and (eq .plane "internal") (eq (include "radius.authz.tls.provider" .root) "external"))) -}}
{{- fail "Requested issuer plane is absent from the CA lifecycle." -}}
{{- end -}}
{{- if $source -}}
{{- $source.issuer | toJson -}}
{{- else -}}
{{- $c := .root.Values.global.rbac.certificates | default dict -}}
{{- if eq (include "radius.authz.certificates.mode" .root) "issuerRef" -}}
{{- $c.issuerRef | toJson -}}
{{- else -}}
{{- $name := "radius-ca-issuer" -}}
{{- if eq .plane "aggregation" -}}{{- $name = "radius-apiserver-issuer" -}}{{- end -}}
{{- dict "group" "cert-manager.io" "kind" "Issuer" "name" $name | toJson -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "radius.authz.certificates.receivers" -}}
{{- if eq (include "radius.authz.tls.provider" .) "radius" -}}
{{- include "radius.authz.spiffe.receivers" . -}}
{{- else -}}[]{{- end -}}
{{- end -}}

{{- define "radius.authz.certificates.validate" -}}
{{- $c := .Values.global.rbac.certificates | default dict -}}
{{- $mode := include "radius.authz.certificates.mode" . -}}
{{- $lifecycle := include "radius.lifecycleSnapshot" . | mustFromJson -}}
{{- if eq $mode "selfSigned" -}}
{{- if or $c.caSecret $c.caBundle (($c.issuerRef | default dict).name) -}}
{{- fail "selfSigned must not specify caSecret, caBundle or issuerRef.name; CA/mode switches require the certificate lifecycle." -}}
{{- end -}}
{{- else -}}
{{- if or (not (kindIs "string" $c.caBundle)) (not (regexMatch `^-----BEGIN CERTIFICATE-----\r?\n[A-Za-z0-9+/=\r\n]+-----END CERTIFICATE-----\s*$` (trim ($c.caBundle | default "")))) -}}
{{- fail "global.rbac.certificates.caBundle must contain exactly one public dedicated CA certificate in PEM format, not a private key or organization chain." -}}
{{- end -}}
{{- if eq $mode "caSecret" -}}
{{- if or (not (kindIs "string" $c.caSecret)) (not (regexMatch `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` (toString $c.caSecret))) (gt (len (toString $c.caSecret)) 253) -}}
{{- fail "global.rbac.certificates.caSecret must be the name of an existing operator-owned TLS Secret in the release namespace." -}}
{{- end -}}
{{- if has $c.caSecret (list "radius-ca" "radius-apiserver-ca" "radius-apiserver-trust" "ucp-cert" "ucp-apiserver-tls" "ucp-spiffe" "applications-rp-spiffe" "dynamic-rp-spiffe" "bicep-de-spiffe") -}}
{{- fail "global.rbac.certificates.caSecret must not name a Radius-managed certificate Secret. Use a separate operator-owned signing Secret." -}}
{{- end -}}
{{- if ($c.issuerRef | default dict).name -}}{{- fail "caSecret must not specify issuerRef.name." -}}{{- end -}}
{{- else -}}
{{- if $c.caSecret -}}{{- fail "issuerRef must not specify caSecret; its private key stays with the existing issuer." -}}{{- end -}}
{{- $issuer := $c.issuerRef | default dict -}}
{{- if or (ne $issuer.group "cert-manager.io") (not (has $issuer.kind (list "Issuer" "ClusterIssuer"))) (not (kindIs "string" $issuer.name)) (not (regexMatch `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` (toString $issuer.name))) -}}
{{- fail "issuerRef requires issuerRef.group=cert-manager.io, kind=Issuer or ClusterIssuer, and an existing issuer name." -}}
{{- end -}}
{{- $issuerNamespace := "" -}}
{{- if eq $issuer.kind "Issuer" -}}{{- $issuerNamespace = .Release.Namespace -}}{{- end -}}
{{- $existing := lookup "cert-manager.io/v1" $issuer.kind $issuerNamespace $issuer.name -}}
{{- if not $existing -}}{{- fail "issuerRef prerequisite: the configured existing Issuer/ClusterIssuer is missing; use a live cluster-connected Helm render." -}}{{- end -}}
{{- if eq $issuer.kind "Issuer" -}}
{{- $signer := dig "spec" "ca" "secretName" "" $existing -}}
{{- if and $signer (or (and .Values.global.rootCA.cert (eq .Values.global.rootCA.secretName $signer))
    (and .Values.global.targetCluster.enabled (eq .Values.global.targetCluster.secretName $signer))) -}}
{{- fail (printf "A shared workload mount must not use the selected Issuer's signing Secret %q." $signer) -}}
{{- end -}}
{{- end -}}
{{- $ready := false -}}
{{- range dig "status" "conditions" list $existing -}}
{{- if and (eq .type "Ready") (eq .status "True") (gt (int64 $existing.metadata.generation) 0) (eq (int64 .observedGeneration) (int64 $existing.metadata.generation)) -}}
{{- $ready = true -}}
{{- end -}}
{{- end -}}
{{- if or (not $ready) $existing.metadata.deletionTimestamp -}}
{{- fail (printf "issuerRef prerequisite: %s %s must be Ready at its current generation." $issuer.kind $issuer.name) -}}
{{- end -}}
{{- $leaves := list "ucp-cert" -}}
{{- range $receiver := include "radius.authz.certificates.receivers" . | fromJsonArray -}}
{{- $leaves = append $leaves (printf "%s-spiffe" $receiver) -}}
{{- end -}}
{{- range $name := $leaves -}}
{{- $leaf := lookup "cert-manager.io/v1" "Certificate" $.Release.Namespace $name -}}
{{- if and $leaf (not (deepEqual (dig "spec" "issuerRef" dict $leaf) $issuer)) -}}
{{- fail (printf "certificate lifecycle prerequisite: Certificate %s issuerRef differs from the selected issuer; use the lifecycle command." $name) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- if not $lifecycle -}}
{{- range $plane := list "internal" "aggregation" -}}
{{- if or (eq $plane "aggregation") (eq (include "radius.authz.tls.provider" $) "radius") -}}
{{- $name := "radius-ca-issuer" -}}{{- $signing := "radius-ca" -}}
{{- if eq $plane "aggregation" -}}{{- $name = "radius-apiserver-issuer" -}}{{- $signing = "radius-apiserver-ca" -}}{{- end -}}
{{- $existing := lookup "cert-manager.io/v1" "Issuer" $.Release.Namespace $name -}}
{{- if $existing -}}
{{- $expected := $signing -}}{{- if eq $mode "caSecret" -}}{{- $expected = $c.caSecret -}}{{- end -}}
{{- if or (eq $mode "issuerRef") (ne (dig "spec" "ca" "secretName" "" $existing) $expected) -}}
{{- fail (printf "certificate lifecycle prerequisite: existing Issuer %s would be removed or retargeted; use the lifecycle command instead of changing certificate modes through Helm." $name) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Return the selected public source, identity, digests and issuer for each plane.
selfSigned aggregation follows the owned signer's ca.crt; existing BYO public
sources stay pinned. Overlap and mode switching belong to the lifecycle.
*/}}
{{- define "radius.authz.certificates.publicSource" -}}
{{- $root := .root -}}
{{- $lifecycle := include "radius.lifecycleSnapshot" $root | mustFromJson -}}
{{- if $lifecycle -}}
{{- $source := index $lifecycle .plane -}}
{{- if not $source -}}{{- fail "Requested trust plane is absent from the CA lifecycle." -}}{{- end -}}
{{- $source | toJson -}}
{{- else -}}
{{- $mode := include "radius.authz.certificates.mode" $root -}}
{{- $kind := "ConfigMap" -}}{{- $name := "radius-mtls-ca" -}}{{- $signing := "radius-ca" -}}
{{- if eq .plane "aggregation" -}}
{{- $kind = "Secret" -}}{{- $name = "radius-apiserver-trust" -}}{{- $signing = "radius-apiserver-ca" -}}
{{- if eq $mode "selfSigned" -}}
{{- $pin := lookup "v1" "Secret" $root.Release.Namespace $name -}}
{{- $annotations := dig "metadata" "annotations" dict $pin -}}
{{- if and (hasKey $annotations "radius.io/certificate-mode") (ne (index $annotations "radius.io/certificate-mode") $mode) -}}
{{- fail "certificate lifecycle prerequisite: radius-apiserver-trust belongs to another certificate mode; use the lifecycle command before switching to selfSigned." -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- if eq $mode "selfSigned" -}}{{- $kind = "Secret" -}}{{- $name = $signing -}}{{- end -}}
{{- $existing := lookup "v1" $kind $root.Release.Namespace $name -}}
{{- $observed := $existing -}}
{{- $bundle := "" -}}
{{- if and (eq $mode "selfSigned") $existing -}}
{{- $annotations := $existing.metadata.annotations | default dict -}}
{{- $data := $existing.data | default dict -}}
{{- if or (ne $existing.type "kubernetes.io/tls")
  (ne (index $annotations "cert-manager.io/certificate-name") $signing)
  (ne (index $annotations "cert-manager.io/issuer-name") "radius-selfsigned-bootstrap")
  (ne (index $annotations "cert-manager.io/issuer-kind") "Issuer")
  (ne (index $annotations "cert-manager.io/issuer-group") "cert-manager.io")
  (empty (index $data "tls.crt")) (empty (index $data "tls.key")) (empty (index $data "ca.crt")) -}}
{{- fail (printf "selfSigned %s prerequisite: %s must be a TLS Secret issued for its Certificate by cert-manager.io/Issuer radius-selfsigned-bootstrap with tls.crt, tls.key and ca.crt; preserve foreign resources and repair ownership before retrying." .plane $signing) -}}
{{- end -}}
{{- end -}}
{{- if ne $mode "selfSigned" -}}
{{- $bundle = printf "%s\n" (trim $root.Values.global.rbac.certificates.caBundle) -}}
{{- else if eq .plane "internal" -}}
{{- $secret := lookup "v1" "Secret" $root.Release.Namespace $signing -}}
{{- $observed = $secret -}}
{{- $bundle = dig "data" "ca.crt" "" $secret | b64dec -}}
{{- else -}}
{{- $bundle = dig "data" "ca.crt" "" $existing | b64dec -}}
{{- end -}}
{{- if and $existing (ne $mode "selfSigned") -}}
{{- $recordedMode := index ($existing.metadata.annotations | default dict) "radius.io/certificate-mode" | default "" -}}
{{- $annotations := $existing.metadata.annotations | default dict -}}
{{- if and (eq .plane "aggregation") (eq $mode "issuerRef") (hasKey $annotations "radius.io/issuer-ref") (ne (index $annotations "radius.io/issuer-ref") (include "radius.authz.certificates.issuer" .)) -}}
{{- fail "certificate lifecycle prerequisite: retained aggregation trust issuerRef differs from the selected issuer; use the lifecycle command." -}}
{{- end -}}
{{- if and $recordedMode (ne $recordedMode $mode) -}}
{{- fail "certificate lifecycle prerequisite: the public source belongs to another certificate mode; use the lifecycle command." -}}
{{- end -}}
{{- $actual := index ($existing.data | default dict) "ca.crt" | default "" -}}
{{- if eq $kind "Secret" -}}{{- $actual = b64dec $actual -}}{{- end -}}
{{- if or (ne (len ($existing.data | default dict)) 1) (not (empty $existing.binaryData)) (not (empty $existing.stringData)) (empty $actual) (ne (trim $actual) (trim $bundle)) -}}
{{- fail (printf "certificate lifecycle prerequisite: public %s %s is empty, contains other data, or differs from the selected CA; preserve it and use the lifecycle command." $kind $name) -}}
{{- end -}}
{{- if ne $mode "selfSigned" -}}{{- $bundle = $actual -}}{{- end -}}
{{- end -}}
{{- if and (eq .plane "internal") (eq $mode "selfSigned") -}}
{{- $kind = "Secret" -}}{{- $name = "radius-ca" -}}
{{- end -}}
{{- $issuer := include "radius.authz.certificates.issuer" . | fromJson -}}
{{- dict "kind" $kind "name" $name "key" "ca.crt" "bundle" $bundle
  "uid" (dig "metadata" "uid" "" $observed) "resourceVersion" (dig "metadata" "resourceVersion" "" $observed)
  "bootstrapUID" (index (dig "metadata" "annotations" (dict) $observed) "radius.io/certificate-bootstrap" | default "")
  "contentDigest" (sha256sum $bundle) "issuerDigest" (sha256sum (toJson $issuer))
  "digest" (sha256sum (printf "%s\n%s" $bundle (toJson $issuer))) "issuer" $issuer | toJson -}}
{{- end -}}
{{- end -}}
