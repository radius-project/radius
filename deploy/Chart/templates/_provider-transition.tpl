{{/*
Preparation isolates configuration without changing shared token projections,
discovery permissions, or the aggregation certificate lifecycle.
*/}}
{{- define "radius.provider.configurationName" -}}
{{- $generation := (.root.Values.global.rbac | default dict).providerGeneration | default "" -}}
{{- if and $generation (not (regexMatch `^[0-9a-f]{32}$` $generation)) -}}
{{- fail "providerGeneration requires the exact token/CSI transition generation." -}}
{{- end -}}
{{- .name -}}{{- if $generation -}}-{{ $generation }}{{- end -}}
{{- end -}}

{{- define "radius.provider.replicas" -}}
{{- $counts := (.root.Values.global.rbac | default dict).providerReplicas | default dict -}}
{{- if $counts -}}
{{- if or (ne (len $counts) 5) (not (hasKey $counts .name)) -}}
{{- fail "providerReplicas requires all five pinned workloads, including the trust-only controller." -}}
{{- end -}}
{{- $value := index $counts .name | toString -}}
{{- if or (not (regexMatch `^[1-9][0-9]*$` $value)) (and (eq .name "controller") (ne $value "1")) -}}
{{- fail "providerReplicas requires positive replica counts and exactly one controller." -}}
{{- end -}}
{{- $value -}}
{{- else -}}1{{- end -}}
{{- end -}}

{{- define "radius.internalIdentity.podEnv" -}}
{{- if ne (include "radius.authz.mode" .) "off" }}
- name: RADIUS_POD_UID
  valueFrom:
    fieldRef:
      fieldPath: metadata.uid
{{- end -}}
{{- end -}}
