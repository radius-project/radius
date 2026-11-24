{{/*
Steady-state certificate contract. A10 replaces the journal guard with its validated
snapshot resolver; an ordinary Helm render must never overwrite staged overlap.
*/}}
{{/*
Consume one field of an already parsed DER certificate. Helm's buildCustomCert
checks the X.509 encoding but doesn't expose its subject or public key.
*/}}
{{- define "radius.authz.certificates.readField" -}}
{{- $der := .der -}}{{- $i := .offset -}}
{{- $length := int (index $der (add $i 1)) -}}
{{- $i = int (add $i 2) -}}
{{- if ge $length 128 -}}
{{- $count := int (sub $length 128) -}}
{{- $length = 0 -}}
{{- range $j := until $count -}}
{{- $length = int (add (mul $length 256) (int (index $der (add $i $j)))) -}}
{{- end -}}
{{- $i = int (add $i $count) -}}
{{- end -}}
{{- $end := int (add $i $length) -}}
{{- $_ := set . "offset" $end -}}
{{- substr $i $end $der -}}
{{- end -}}

{{/*
Compare the same two encoded fields as the CLI's RawSubject and
RawSubjectPublicKeyInfo. Dates, serial number and signature change on renewal.
This only extracts identity from the public material; it does not verify issuance.
*/}}
{{- define "radius.authz.certificates.subjectAndKey" -}}
{{- $cursor := dict "der" . "offset" 0 -}}
{{- $cert := include "radius.authz.certificates.readField" $cursor -}}
{{- $cursor = dict "der" $cert "offset" 0 -}}
{{- $tbs := include "radius.authz.certificates.readField" $cursor -}}
{{- $cursor = dict "der" $tbs "offset" 0 -}}
{{- if and $tbs (eq (int (index $tbs 0)) 160) -}}
{{- $_ := include "radius.authz.certificates.readField" $cursor -}}
{{- end -}}
{{- /* Serial number, signature algorithm, issuer and validity. */ -}}
{{- range until 4 -}}
{{- $_ := include "radius.authz.certificates.readField" $cursor -}}
{{- end -}}
{{- $start := $cursor.offset -}}
{{- range until 2 -}}
{{- $_ := include "radius.authz.certificates.readField" $cursor -}}
{{- end -}}
{{- substr $start $cursor.offset $tbs | b64enc -}}
{{- end -}}

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
{{- $retained := dict -}}
{{- if eq .plane "aggregation" -}}
{{- $kind = "Secret" -}}{{- $name = "radius-apiserver-trust" -}}{{- $signing = "radius-apiserver-ca" -}}
{{- if eq $mode "selfSigned" -}}
{{- $pin := lookup "v1" "Secret" $root.Release.Namespace $name -}}
{{- $annotations := dig "metadata" "annotations" dict $pin -}}
{{- if and (hasKey $annotations "radius.io/certificate-mode") (ne (index $annotations "radius.io/certificate-mode") $mode) -}}
{{- fail "certificate lifecycle prerequisite: radius-apiserver-trust belongs to another certificate mode; use the lifecycle command before switching to selfSigned." -}}
{{- end -}}
{{- if hasKey $annotations "radius.io/certificate-mode" -}}{{- $retained = $pin -}}{{- end -}}
{{- end -}}
{{- end -}}
{{- if eq $mode "selfSigned" -}}{{- $kind = "Secret" -}}{{- $name = $signing -}}{{- end -}}
{{- $existing := lookup "v1" $kind $root.Release.Namespace $name -}}
{{- if and $retained (not $existing) -}}
{{- fail "selfSigned aggregation continuity: radius-apiserver-trust exists without radius-apiserver-ca; preserve the pin and restore the original signer before retrying." -}}
{{- end -}}
{{- $observed := $existing -}}
{{- $bundle := "" -}}
{{- if and (eq $mode "selfSigned") $existing -}}
{{- $annotations := $existing.metadata.annotations | default dict -}}
{{- $data := $existing.data | default dict -}}
{{- if and (eq .plane "aggregation") (not (and (eq (get $annotations "radapp.io/release-name") $root.Release.Name) (eq (get $annotations "radapp.io/release-namespace") $root.Release.Namespace))) -}}
{{- fail (printf "Secret %s/radius-apiserver-ca is not marked for Helm release %s/%s with annotations radapp.io/release-name and radapp.io/release-namespace, so the release will not trust its CA. Preserve the signer and verify its ownership before retrying." $root.Release.Namespace $root.Release.Namespace $root.Release.Name) -}}
{{- end -}}
{{- $caPEM := get $data "ca.crt" | default "" | b64dec -}}
{{- if and $caPEM (or (not (contains "-----BEGIN CERTIFICATE-----" $caPEM)) (trim (regexReplaceAll "(?s)-----BEGIN CERTIFICATE-----\\r?\\n[A-Za-z0-9+/=\\r\\n]*-----END CERTIFICATE-----" $caPEM ""))) -}}
{{- fail (printf "ca.crt of Secret %s/%s holds something other than PEM certificates, so the release will not trust it. Restore the original signer from backup before retrying." $root.Release.Namespace $signing) -}}
{{- end -}}
{{- if or (ne $existing.type "kubernetes.io/tls")
  (ne (index $annotations "cert-manager.io/certificate-name") $signing)
  (ne (index $annotations "cert-manager.io/issuer-name") "radius-selfsigned-bootstrap")
  (ne (index $annotations "cert-manager.io/issuer-kind") "Issuer")
  (ne (index $annotations "cert-manager.io/issuer-group") "cert-manager.io")
  (empty (index $data "tls.crt")) (empty (index $data "tls.key")) (empty (index $data "ca.crt")) -}}
{{- fail (printf "selfSigned %s prerequisite: %s must be a TLS Secret issued for its Certificate by cert-manager.io/Issuer radius-selfsigned-bootstrap with tls.crt, tls.key and ca.crt; preserve foreign resources and repair ownership before retrying." .plane $signing) -}}
{{- end -}}
{{- if $retained -}}
{{- $identity := "" -}}
{{- range $encoded := list (dig "data" "ca.crt" "" $retained) (index $data "tls.crt") (index $data "ca.crt") -}}
{{- $pem := trim (b64dec $encoded) -}}
{{- $pattern := `-----BEGIN CERTIFICATE-----\r?\n[A-Za-z0-9+/=\r\n]+\r?\n-----END CERTIFICATE-----` -}}
{{- if not (regexMatch (printf `^(%s)([ \t\r\n]+%s)*$` $pattern $pattern) $pem) -}}
{{- fail "selfSigned aggregation continuity: expected only public certificate PEM blocks in the retained pin and signer." -}}
{{- end -}}
{{- range $block := regexFindAll $pattern $pem -1 -}}
{{- /* Parse each complete X.509 certificate before reading DER fields. Never emit the returned key pair. */ -}}
{{- $_ := buildCustomCert (b64enc $block) (index $data "tls.key") -}}
{{- $der := regexReplaceAll `-----[A-Z ]+-----|\s+` $block "" | b64dec -}}
{{- $next := include "radius.authz.certificates.subjectAndKey" $der -}}
{{- if and $identity (ne $next $identity) -}}
{{- fail "selfSigned aggregation continuity: the signer key or subject differs from the retained pin; preserve both Secrets and use the certificate lifecycle." -}}
{{- end -}}
{{- $identity = $next -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- if ne $mode "selfSigned" -}}
{{- $bundle = printf "%s\n" (trim $root.Values.global.rbac.certificates.caBundle) -}}
{{- else if eq .plane "internal" -}}
{{- $secret := lookup "v1" "Secret" $root.Release.Namespace $signing -}}
{{- $observed = $secret -}}
{{- $bundle = dig "data" "ca.crt" "" $secret | b64dec -}}
{{- $copy := lookup "v1" "ConfigMap" $root.Release.Namespace "radius-mtls-ca" -}}
{{- if and $copy (or (ne (len ($copy.data | default dict)) 1) (not (empty $copy.binaryData))
  (ne (trim (dig "data" "ca.crt" "" $copy)) (trim $bundle))) -}}
{{- fail "The retained internal ConfigMap differs from the selected CA; restore the pinned source before retrying." -}}
{{- end -}}
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
{{/* Compare signer material to the validated journal bundle independently of PEM wrapping or order. */}}
{{- define "radius.caSet" -}}
{{- $block := `-----BEGIN CERTIFICATE-----\r?\n([A-Za-z0-9+/=\r\n]+)\r?\n-----END CERTIFICATE-----` -}}
{{- if not (regexMatch (printf `^[ \t\r\n]*%s([ \t\r\n]+%s)*[ \t\r\n]*$` $block $block) .) -}}
{{- fail "Restore the recorded internal CA set: invalid certificate bundle." -}}
{{- end -}}
{{- $seen := dict -}}
{{- range regexFindAll $block . -1 -}}
{{- $encoded := regexReplaceAll `[\r\n]` (regexReplaceAll $block . "${1}") "" -}}
{{- $der := b64dec $encoded -}}
{{- if or (ne (b64enc $der) $encoded) (hasKey $seen $encoded) -}}
{{- fail "Restore the recorded internal CA set: malformed or duplicate certificate." -}}
{{- end -}}
{{- $_ := set $seen $encoded true -}}
{{- end -}}
{{- keys $seen | sortAlpha | toJson -}}
{{- end -}}
