{{- define "radius.authz.certificates.bootstrapIssuer" -}}
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: radius-selfsigned-bootstrap
  namespace: {{ .Release.Namespace }}
  labels:
    app.kubernetes.io/part-of: radius
  {{- include "radius.initialCertificateAnnotation" . | nindent 2 }}
spec:
  selfSigned: {}
{{- end -}}

{{- define "radius.authz.certificates.root" -}}
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: {{ .name }}
  namespace: {{ .root.Release.Namespace }}
  labels:
    app.kubernetes.io/part-of: radius
  {{- include "radius.initialCertificateAnnotation" .root | nindent 2 }}
spec:
  isCA: true
  commonName: {{ .name }}
  secretName: {{ .name }}
  duration: 8760h
  renewBefore: 2920h
  privateKey:
    algorithm: ECDSA
    size: 256
    rotationPolicy: Never
  issuerRef:
    group: cert-manager.io
    kind: Issuer
    name: radius-selfsigned-bootstrap
{{- end -}}

{{- define "radius.initialCertificateAnnotation" -}}
{{- $journal := lookup "v1" "ConfigMap" .Release.Namespace "radius-certificate-bootstrap" -}}
{{- if $journal -}}
annotations:
  radius.io/certificate-bootstrap: {{ $journal.metadata.uid | quote }}
{{- end -}}
{{- end -}}

{{- define "radius.authz.certificates.signingIssuer" -}}
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: {{ .name }}
  namespace: {{ .root.Release.Namespace }}
  labels:
    app.kubernetes.io/part-of: radius
spec:
  ca:
    secretName: {{ .secretName | quote }}
{{- end -}}
