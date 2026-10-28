{{- define "radius.versiontag" -}}
{{- .Chart.AppVersion -}}
{{- end -}}

{{/* Validate the startup-loaded Flux settings before changing the controller. */}}
{{- define "radius.controller.flux" -}}
{{- if and (hasKey .Values.controller "flux") (not (kindIs "map" .Values.controller.flux)) -}}
  {{- fail "controller.flux must be an object" -}}
{{- end -}}
{{- $flux := .Values.controller.flux | default dict -}}
{{- if not (kindIs "map" $flux) -}}{{ fail "controller.flux must be an object" }}{{- end -}}
{{- range $key, $_ := $flux -}}
  {{- if not (has $key (list "enabled" "sources")) -}}{{ fail (printf "unknown controller.flux field %s" $key) }}{{- end -}}
{{- end -}}
{{- if and (hasKey $flux "enabled") (not (kindIs "bool" $flux.enabled)) -}}
  {{- fail "controller.flux.enabled must be a boolean" -}}
{{- end -}}
{{- if and (hasKey $flux "sources") (not (kindIs "slice" $flux.sources)) -}}
  {{- fail "controller.flux.sources must be a list" -}}
{{- end -}}
{{- $sources := $flux.sources | default list -}}
{{- if not (kindIs "slice" $sources) -}}{{ fail "controller.flux.sources must be a list" }}{{- end -}}
{{- $seen := dict -}}
{{- range $i, $source := $sources -}}
  {{- if not (kindIs "map" $source) -}}{{ fail "controller.flux.sources entries must be objects" }}{{- end -}}
  {{- range $key, $_ := $source -}}
    {{- if not (has $key (list "namespace" "name" "uid")) -}}{{ fail (printf "unknown controller.flux.sources field %s" $key) }}{{- end -}}
  {{- end -}}
  {{- if not (kindIs "string" $source.namespace) -}}{{ fail "controller.flux.sources namespace must be a string" }}{{- end -}}
  {{- if or (gt (len $source.namespace) 63) (not (regexMatch "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$" $source.namespace)) -}}
    {{- fail "controller.flux.sources namespace must be a Kubernetes namespace" -}}
  {{- end -}}
  {{- if not (kindIs "string" $source.name) -}}{{ fail "controller.flux.sources name must be a string" }}{{- end -}}
  {{- if or (gt (len $source.name) 253) (not (regexMatch "^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$" $source.name)) -}}
    {{- fail "controller.flux.sources name must be a Kubernetes object name" -}}
  {{- end -}}
  {{- if not (kindIs "string" $source.uid) -}}{{ fail "controller.flux.sources uid must be a string" }}{{- end -}}
  {{- if or (eq $source.uid "") (regexMatch "[\\s*/\\\\]" $source.uid) -}}
    {{- fail "controller.flux.sources uid must be a nonempty exact UID" -}}
  {{- end -}}
  {{- $key := printf "%s/%s" $source.namespace $source.name -}}
  {{- if hasKey $seen $key -}}{{ fail (printf "controller.flux.sources contains duplicate source %s" $key) }}{{- end -}}
  {{- $_ := set $seen $key true -}}
{{- end -}}
{{- toYaml (dict "enabled" ($flux.enabled | default false) "sources" $sources) -}}
{{- end -}}

{{/* External images retain their independently published latest tag for edge charts. */}}
{{- define "radius.externalversiontag" -}}
{{- $version := . | toString -}}
{{- if eq $version "edge" -}}latest{{- else -}}{{ $version }}{{- end -}}
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
{{- $encode := .encodeDefault | default true -}}
{{- if and $secretData (hasKey $secretData .key) -}}
  {{- $value = index $secretData .key -}}
{{- else if .defaultValue -}}
  {{- if $encode -}}
    {{- $value = .defaultValue | toString | b64enc -}}
  {{- else -}}
    {{- $value = .defaultValue | toString -}}
  {{- end -}}
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
  - image - String - Required - Image name (e.g., "controller" or "myregistry.io/custom-image")
  - tag - String - Required - Image tag (can be component-specific, global, or default)
  - global - Object - Required - Global values containing imageRegistry and imageTag

Priority for registry:
1. If image appears to be a full registry path (contains domain or port before first /), use it as-is with tag handling
2. If global.imageRegistry is set, use it as the registry
3. Otherwise, use ghcr.io/radius-project as the default registry

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
  {{- /* Not a full path, no global registry, use default ghcr.io/radius-project */ -}}
  ghcr.io/radius-project/{{ .image }}:{{ .tag }}
{{- end -}}
{{- end -}}

{{/*
Render the external target-cluster kubeconfig volume for recipe-executing pods.

When global.targetCluster.enabled is true, recipe execution (the Terraform
kubernetes provider in dynamic-rp and the Bicep extension via the Deployment
Engine) targets the external cluster described by a mounted kubeconfig instead of
the control-plane cluster. The Secret holding the kubeconfig is owned outside the
chart (for example a CI workflow that mints and refreshes it). When disabled,
nothing is rendered and behavior is unchanged.

Usage (pass the root context):
  {{- include "radius.targetCluster.volume" . | nindent 8 }}
  {{- include "radius.targetCluster.volumeMount" . | nindent 8 }}
  {{- include "radius.targetCluster.env" . | nindent 8 }}
*/}}
{{- define "radius.targetCluster.volume" -}}
{{- if .Values.global.targetCluster.enabled -}}
- name: target-kubeconfig
  secret:
    secretName: {{ .Values.global.targetCluster.secretName }}
    # World-readable (0444) so the non-root RP/DE containers can read the mounted
    # kubeconfig. Secret files are owned by root, so 0400 (root-only) would be
    # unreadable to a non-root process; the secret is already isolated to the pod,
    # so 0444 does not widen the real security boundary.
    defaultMode: 0444
{{- end -}}
{{- end -}}

{{- define "radius.targetCluster.volumeMount" -}}
{{- if .Values.global.targetCluster.enabled -}}
- name: target-kubeconfig
  mountPath: {{ .Values.global.targetCluster.mountPath }}
  readOnly: true
{{- end -}}
{{- end -}}

{{- define "radius.targetCluster.env" -}}
{{- if .Values.global.targetCluster.enabled -}}
- name: RADIUS_TARGET_KUBECONFIG
  value: "{{ .Values.global.targetCluster.mountPath }}/{{ .Values.global.targetCluster.secretKey }}"
{{- end -}}
{{- end -}}

{{/*
Returns the authorization mode (off, dryRun, or enforce) derived from global.rbac.
Fails rendering when dry run is combined with enforcement.
*/}}
{{- define "radius.authz.mode" -}}
{{- $rbac := .Values.global.rbac | default dict -}}
{{- if and $rbac.enabled $rbac.dryRun -}}
{{- fail "global.rbac.dryRun=true cannot be combined with global.rbac.enabled=true: dry run is a preflight check for installations that are not yet enforcing. Set global.rbac.enabled=false to run the dry run." -}}
{{- else if $rbac.dryRun -}}
dryRun
{{- else if $rbac.enabled -}}
enforce
{{- else -}}
off
{{- end -}}
{{- end -}}
