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
