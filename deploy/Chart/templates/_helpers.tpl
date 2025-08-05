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

Params:
  - secret - String - Required - Name of the 'Secret' resource where the password is stored.
  - namespace - String - Required - Namespace of the 'Secret' resource where the password is stored.
  - key - String - Required - Name of the key in the secret.
  - defaultValue - String - Required - Default value to use if the secret does not exist.

References:
  - https://github.com/bitnami/charts/blob/main/bitnami/common/templates/_secrets.tpl
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

{{/*
Create a fully qualified image name with optional registry and tag overrides.

Usage:
{{ include "radius.image" (dict "image" .Values.component.image "tag" (.Values.component.tag | default .Values.global.imageTag | default $appversion) "global" .Values.global) }}

Params:
  - image - String - Required - Image name (e.g., "radius-project/controller" or "myregistry.io/custom-image")
  - tag - String - Required - Image tag (can be component-specific, global, or default)
  - global - Object - Required - Global values containing imageRegistry and imageTag

Priority for registry:
1. If image appears to be a full registry path (contains domain or port before first /), use it as-is with tag handling
2. If global.imageRegistry is set, prepend it to the image name
3. Otherwise, use ghcr.io as the default registry

Priority for tag (handled by caller):
1. Component-specific tag (e.g., controller.tag)
2. global.imageTag
3. Chart AppVersion (default)
*/}}
{{- define "radius.image" -}}
{{- $isFullPath := false -}}
{{- /* Check if image looks like a full registry path */ -}}
{{- if contains "/" .image -}}
  {{- $firstPart := (split "/" .image)._0 -}}
  {{- /* Check if first part looks like a registry (has dot or colon, or is localhost) */ -}}
  {{- if or (contains "." $firstPart) (contains ":" $firstPart) (eq $firstPart "localhost") -}}
    {{- $isFullPath = true -}}
  {{- end -}}
{{- end -}}
{{- if $isFullPath -}}
  {{- /* Image is a full path with registry */ -}}
  {{- if contains ":" .image -}}
    {{- /* Check if colon is part of tag (after last /) or port */ -}}
    {{- $parts := splitList "/" .image -}}
    {{- $lastPart := last $parts -}}
    {{- if contains ":" $lastPart -}}
      {{- /* Last part has colon, so image already has a tag */ -}}
      {{- .image }}
    {{- else -}}
      {{- /* Colon is in registry part (port), append tag */ -}}
      {{- .image }}:{{ .tag }}
    {{- end -}}
  {{- else -}}
    {{- /* Full path but no tag, append the provided tag */ -}}
    {{- .image }}:{{ .tag }}
  {{- end -}}
{{- else if .global.imageRegistry -}}
  {{- /* Not a full path, use global registry */ -}}
  {{- .global.imageRegistry }}/{{ .image }}:{{ .tag }}
{{- else -}}
  {{- /* Not a full path, no global registry, use default */ -}}
  ghcr.io/{{ .image }}:{{ .tag }}
{{- end -}}
{{- end -}}
