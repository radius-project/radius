{{/*
Opt-in public component TLS evidence on the existing Go metrics listener.
Deployment-engine proof remains a separate opt-in for compatible engine images.
*/}}
{{- define "radius.authz.startupProofEnv" -}}
{{- $rotation := include "radius.rotationState" . | mustFromJson -}}
{{- $rbac := .Values.global.rbac | default dict -}}
{{- $proof := deepCopy ($rbac.startupProof | default dict) -}}
{{- if and $rotation $proof.enabled -}}
{{- $_ := set $proof "expectedTrustSHA256" $rotation.plan.targetTrustSHA256 -}}
{{- end -}}
{{- $enabled := false -}}
{{- if hasKey $proof "enabled" -}}
{{- $enabled = $proof.enabled -}}
{{- end -}}
{{- if not (kindIs "bool" $enabled) -}}
{{- fail "global.rbac.startupProof.enabled must be a boolean" -}}
{{- end -}}
{{- if $enabled -}}
{{- if eq (include "radius.authz.mode" .) "off" -}}
{{- fail "global.rbac.startupProof.enabled requires component mTLS" -}}
{{- end -}}
{{- if or (not (kindIs "bool" .Values.global.prometheus.enabled)) (not .Values.global.prometheus.enabled) (le (int .Values.global.prometheus.port) 0) (gt (int .Values.global.prometheus.port) 65535) -}}
{{- fail "global.rbac.startupProof.enabled requires the existing metrics listener on a fixed valid port" -}}
{{- end -}}
{{- if eq .Values.global.prometheus.path "/healthz/ucp-mtls" -}}
{{- fail "global.prometheus.path conflicts with the TLS startup proof endpoint" -}}
{{- end -}}
{{- if or (not (kindIs "string" $proof.expectedTrustSHA256)) (not (regexMatch `^[0-9a-f]{64}$` (toString $proof.expectedTrustSHA256))) -}}
{{- fail "global.rbac.startupProof.expectedTrustSHA256 must be a lowercase SHA-256 digest of the dedicated CA fingerprint set" -}}
{{- end }}
- name: UCP_MTLS_STARTUP_PROOF_ENABLED
  value: "true"
- name: UCP_MTLS_EXPECTED_TRUST_SHA256
  value: {{ $proof.expectedTrustSHA256 | quote }}
- name: UCP_MTLS_POD_UID
  valueFrom:
    fieldRef:
      fieldPath: metadata.uid
{{- else if $proof.expectedTrustSHA256 -}}
{{- fail "global.rbac.startupProof.expectedTrustSHA256 requires startupProof.enabled=true" -}}
{{- end -}}
{{- end -}}
