{{/* Parse version and extract major and manor version from Appversion for image tag. */}}
{{- define "radius.versiontag" }}
{{- $version := .Chart.AppVersion }}
{{- /* Set latest if version is edge */}}
{{- if eq $version "edge" }}
  {{- $version = "latest" }}
{{- end -}}
{{- /* Tag version will be 'major.minor' unless version is latest or rc release */}}
{{- if and (ne $version "latest") (not (contains "rc" $version)) }}
  {{- $ver := split "." $version }}
  {{- $version = printf "%s.%s" $ver._0 $ver._1 }}
{{- end -}}
{{- print $version }}
{{- end -}}

 

{{/*
Reuses the value from an existing secret, otherwise sets its value to a default value.

Usage:
{{ include "secrets.lookup" (dict "secret" "secret-name" "namespace" "ns-name" "key" "key-name" "defaultValue" "default-secret") }}
*/}}
{{- define "secrets.lookup" -}}
{{- $value := "" -}}
{{- $namespace := .namespace | toString -}}
{{- $secretData := (lookup "v1" "Secret" $namespace .secret).data -}}
{{- if and $secretData (hasKey $secretData .key) -}}
  {{- $value = index $secretData .key -}}
{{- else if .defaultValue -}}
  {{- $value = .defaultValue | toString | b64enc -}}
{{- end -}}
{{- if $value -}}
{{- printf "%s" $value -}}
{{- end -}}
{{- end -}}
