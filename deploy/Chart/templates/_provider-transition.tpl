{{- define "radius.internalIdentity.podEnv" -}}
{{- if ne (include "radius.authz.mode" .) "off" }}
- name: RADIUS_POD_UID
  valueFrom:
    fieldRef:
      fieldPath: metadata.uid
{{- end -}}
{{- end -}}
