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
{{- $c := .root.Values.global.rbac.certificates | default dict -}}
{{- if eq (include "radius.authz.certificates.mode" .root) "issuerRef" -}}
{{- $c.issuerRef | toJson -}}
{{- else -}}
{{- $name := "radius-ca-issuer" -}}
{{- if eq .plane "aggregation" -}}{{- $name = "radius-apiserver-issuer" -}}{{- end -}}
{{- dict "group" "cert-manager.io" "kind" "Issuer" "name" $name | toJson -}}
{{- end -}}
{{- end -}}

{{- define "radius.authz.certificates.issuerMatch" -}}
{{- printf "(object.spec.issuerRef.name == '%s' && ((!has(object.spec.issuerRef.kind) || object.spec.issuerRef.kind == '') ? 'Issuer' : object.spec.issuerRef.kind) == '%s' && ((!has(object.spec.issuerRef.group) || object.spec.issuerRef.group == '') ? 'cert-manager.io' : object.spec.issuerRef.group) == '%s')" .name .kind .group -}}
{{- end -}}

{{- define "radius.authz.certificates.receivers" -}}
{{- $receivers := list -}}
{{- if eq (include "radius.authz.tls.provider" .) "radius" -}}
{{- $receivers = list "ucp" "applications-rp" "dynamic-rp" -}}
{{- if eq (include "radius.de.internalAuth.enabled" .) "true" -}}
{{- $receivers = append $receivers "bicep-de" -}}
{{- end -}}
{{- end -}}
{{- $receivers | toJson -}}
{{- end -}}

{{- define "radius.authz.certificates.validate" -}}
{{- $c := .Values.global.rbac.certificates | default dict -}}
{{- $mode := include "radius.authz.certificates.mode" . -}}
{{- if lookup "v1" "ConfigMap" .Release.Namespace "radius-ca-rotation" -}}
{{- fail "certificate lifecycle prerequisite: radius-ca-rotation exists; use the lifecycle command that validates and preserves its pinned public sources before rendering Helm." -}}
{{- end -}}
{{- if eq $mode "selfSigned" -}}
{{- if or $c.caSecret $c.caBundle (($c.issuerRef | default dict).name) -}}
{{- fail "selfSigned must not specify caSecret, caBundle or issuerRef.name; CA/mode switches require the certificate lifecycle." -}}
{{- end -}}
{{- else -}}
{{- if not (.Capabilities.APIVersions.Has "admissionregistration.k8s.io/v1/ValidatingAdmissionPolicy") -}}
{{- fail "certificate prerequisite: bring-your-own CA modes require Kubernetes ValidatingAdmissionPolicy support." -}}
{{- end -}}
{{- if or (not (kindIs "string" $c.caBundle)) (not (regexMatch `^-----BEGIN CERTIFICATE-----\r?\n[A-Za-z0-9+/=\r\n]+-----END CERTIFICATE-----\s*$` (trim ($c.caBundle | default "")))) -}}
{{- fail "global.rbac.certificates.caBundle must contain exactly one public dedicated CA certificate in PEM format, not a private key or organization chain." -}}
{{- end -}}
{{- if eq $mode "caSecret" -}}
{{- if or (not (kindIs "string" $c.caSecret)) (not (regexMatch `^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$` (toString $c.caSecret))) (gt (len (toString $c.caSecret)) 253) -}}
{{- fail "global.rbac.certificates.caSecret must be the name of an existing operator-owned TLS Secret in the release namespace." -}}
{{- end -}}
{{- if has $c.caSecret (list "radius-ca" "radius-apiserver-ca" "radius-apiserver-trust" "ucp-cert" "ucp-apiserver-tls" "ucp-mtls" "applications-rp-mtls" "dynamic-rp-mtls" "controller-mtls" "bicep-de-mtls") -}}
{{- fail "global.rbac.certificates.caSecret must not name a Radius-managed certificate Secret. Use a separate operator-owned signing Secret." -}}
{{- end -}}
{{- if ($c.issuerRef | default dict).name -}}{{- fail "caSecret must not specify issuerRef.name." -}}{{- end -}}
{{- else -}}
{{- if $c.caSecret -}}{{- fail "issuerRef must not specify caSecret; its private key stays with the existing issuer." -}}{{- end -}}
{{- include "radius.authz.issuerPolicy.validate" . -}}
{{- end -}}
{{- end -}}
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

{{/*
Return the selected public source, identity, digests and issuer for each plane.
selfSigned aggregation follows the owned signer's ca.crt; existing BYO public
sources stay pinned. Overlap and mode switching belong to the lifecycle.
*/}}
{{- define "radius.authz.certificates.publicSource" -}}
{{- $root := .root -}}
{{- $mode := include "radius.authz.certificates.mode" $root -}}
{{- $kind := "ConfigMap" -}}{{- $name := "radius-mtls-ca" -}}{{- $signing := "radius-ca" -}}
{{- if eq .plane "aggregation" -}}
{{- $kind = "Secret" -}}{{- $name = "radius-apiserver-trust" -}}{{- $signing = "radius-apiserver-ca" -}}
{{- if eq $mode "selfSigned" -}}{{- $name = $signing -}}{{- end -}}
{{- end -}}
{{- $existing := lookup "v1" $kind $root.Release.Namespace $name -}}
{{- $observed := $existing -}}
{{- $bundle := "" -}}
{{- if and (eq .plane "aggregation") (eq $mode "selfSigned") $existing -}}
{{- $annotations := $existing.metadata.annotations | default dict -}}
{{- $data := $existing.data | default dict -}}
{{- if or (ne $existing.type "kubernetes.io/tls")
  (ne (index $annotations "cert-manager.io/certificate-name") "radius-apiserver-ca")
  (ne (index $annotations "cert-manager.io/issuer-name") "radius-selfsigned-bootstrap")
  (ne (index $annotations "cert-manager.io/issuer-kind") "Issuer")
  (ne (index $annotations "cert-manager.io/issuer-group") "cert-manager.io")
  (empty (index $data "tls.crt")) (empty (index $data "tls.key")) (empty (index $data "ca.crt")) -}}
{{- fail "selfSigned aggregation prerequisite: radius-apiserver-ca must be a TLS Secret issued for Certificate radius-apiserver-ca by cert-manager.io/Issuer radius-selfsigned-bootstrap with tls.crt, tls.key and ca.crt; preserve foreign resources and repair ownership before retrying." -}}
{{- end -}}
{{- end -}}
{{- if ne $mode "selfSigned" -}}
{{- $bundle = printf "%s\n" (trim $root.Values.global.rbac.certificates.caBundle) -}}
{{- else if $existing -}}
{{- $bundle = index ($existing.data | default dict) "ca.crt" | default "" -}}
{{- if eq $kind "Secret" -}}{{- $bundle = b64dec $bundle -}}{{- end -}}
{{- else if ne .plane "aggregation" -}}
{{- $secret := lookup "v1" "Secret" $root.Release.Namespace $signing -}}
{{- $observed = $secret -}}
{{- $bundle = dig "data" "ca.crt" "" $secret | b64dec -}}
{{- end -}}
{{- if and $existing (not (and (eq .plane "aggregation") (eq $mode "selfSigned"))) -}}
{{- $recordedMode := index ($existing.metadata.annotations | default dict) "radius.io/certificate-mode" | default "" -}}
{{- if and $recordedMode (ne $recordedMode $mode) -}}
{{- fail "certificate lifecycle prerequisite: the public source belongs to another certificate mode; use the lifecycle command." -}}
{{- end -}}
{{- $actual := index ($existing.data | default dict) "ca.crt" | default "" -}}
{{- if eq $kind "Secret" -}}{{- $actual = b64dec $actual -}}{{- end -}}
{{- if or (ne (len ($existing.data | default dict)) 1) (not (empty $existing.binaryData)) (not (empty $existing.stringData)) (empty $actual) (ne (trim $actual) (trim $bundle)) -}}
{{- fail (printf "certificate lifecycle prerequisite: public %s %s is empty, contains other data, or differs from the selected CA; preserve it and use the lifecycle command." $kind $name) -}}
{{- end -}}
{{- $bundle = $actual -}}
{{- end -}}
{{- if and (eq .plane "internal") (eq $mode "selfSigned") (not $existing) -}}
{{- $kind = "Secret" -}}{{- $name = "radius-ca" -}}
{{- end -}}
{{- $issuer := include "radius.authz.certificates.issuer" . | fromJson -}}
{{- dict "kind" $kind "name" $name "key" "ca.crt" "bundle" $bundle
  "uid" (dig "metadata" "uid" "" $observed) "resourceVersion" (dig "metadata" "resourceVersion" "" $observed)
  "contentDigest" (sha256sum $bundle) "issuerDigest" (sha256sum (toJson $issuer))
  "digest" (sha256sum (printf "%s\n%s" $bundle (toJson $issuer))) "issuer" $issuer | toJson -}}
{{- end -}}
