{{/* Parse version and extract major and manor version from Appversion for image tag. */}}
{{- define "radius.versiontag" }}
{{- $version := .Chart.AppVersion }}
{{- if eq $version "edge" }}
  {{- $version = "latest" }}
{{- end -}}
{{- if ne $version "latest" }}
  {{- $ver := split "." $version }}
  {{- $version = printf "%s.%s" $ver._0 $ver._1 }}
{{- end -}}
{{- print $version }}
{{- end -}}
