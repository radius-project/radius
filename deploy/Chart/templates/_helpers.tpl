{{- define "radius.versiontag" -}}
{{- .Chart.AppVersion -}}
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
radius.controlPlaneReadiness.image renders the full image reference used by the
control-plane readiness hook and the dashboard startup gate. When
controlPlaneReadiness.image is unset, it honors an explicitly configured
preupgrade image; otherwise it uses the pre-upgrade image from the same registry
and tag as UCP so custom and CI builds stay aligned.
*/}}
{{- define "radius.controlPlaneReadiness.image" -}}
{{- $appversion := include "radius.versiontag" . }}
{{- $readiness := .Values.controlPlaneReadiness | default dict }}
{{- $readinessImage := get $readiness "image" | default "" }}
{{- $readinessTag := get $readiness "tag" | default "" }}
{{- if not $readinessImage }}
  {{- if ne .Values.preupgrade.image "pre-upgrade" }}
    {{- $readinessImage = .Values.preupgrade.image }}
    {{- $readinessTag = $readinessTag | default .Values.preupgrade.tag }}
  {{- else }}
    {{- $ucpImageName := last (splitList "/" .Values.ucp.image) }}
    {{- $ucpEmbeddedTag := "" }}
    {{- if contains ":" $ucpImageName }}
      {{- $ucpEmbeddedTag = regexReplaceAll "^.*:" $ucpImageName "" }}
    {{- end }}
    {{- if contains "/" .Values.ucp.image }}
      {{- $readinessImage = regexReplaceAll "/[^/]+$" .Values.ucp.image "/pre-upgrade" }}
    {{- else }}
      {{- $readinessImage = "pre-upgrade" }}
    {{- end }}
    {{- $readinessTag = $readinessTag | default $ucpEmbeddedTag | default .Values.ucp.tag }}
  {{- end }}
{{- end }}
{{- $readinessTag = $readinessTag | default .Values.global.imageTag | default $appversion }}
{{- include "radius.image" (dict "image" $readinessImage "tag" $readinessTag "global" .Values.global) }}
{{- end -}}

{{/*
radius.dashboard.startupGate.deployments lists the core control-plane Deployments,
comma separated, that must be available before the dashboard container image is pulled.
*/}}
{{- define "radius.dashboard.startupGate.deployments" -}}
ucp,applications-rp,dynamic-rp,controller,bicep-de
{{- end -}}

{{/*
radius.terraform.image renders the image for the terraform-init init container.
global.terraform.image and global.terraform.tag override it. Otherwise it uses the
terraform-init image from the same registry and tag as applications-rp, so custom and
CI builds stay aligned. It deliberately ignores the preupgrade and controlPlaneReadiness
overrides: those images may be pinned to versions that cannot download Terraform.
*/}}
{{- define "radius.terraform.image" -}}
{{- $appversion := include "radius.versiontag" . }}
{{- $image := .Values.global.terraform.image | default "" }}
{{- $tag := .Values.global.terraform.tag | default "" }}
{{- if not $image }}
  {{- $rpImageName := last (splitList "/" .Values.rp.image) }}
  {{- $rpEmbeddedTag := "" }}
  {{- if contains ":" $rpImageName }}
    {{- $rpEmbeddedTag = regexReplaceAll "^.*:" $rpImageName "" }}
  {{- end }}
  {{- if contains "/" .Values.rp.image }}
    {{- $image = regexReplaceAll "/[^/]+$" .Values.rp.image "/terraform-init" }}
  {{- else }}
    {{- $image = "terraform-init" }}
  {{- end }}
  {{- $tag = $tag | default $rpEmbeddedTag | default .Values.rp.tag }}
{{- end }}
{{- $tag = $tag | default .Values.global.imageTag | default $appversion }}
{{- include "radius.image" (dict "image" $image "tag" $tag "global" .Values.global) }}
{{- end -}}

{{/*
radius.terraform.initContainer renders the terraform-init init container, which downloads
Terraform to the shared terraform volume before a resource provider starts. The download runs
outside the resource provider container, and a failed download blocks the pod.

Usage:
  {{- include "radius.terraform.initContainer" (dict "root" . "path" .Values.rp.terraform.path) | nindent 6 }}
*/}}
{{- define "radius.terraform.initContainer" -}}
{{- $root := .root -}}
{{- $tf := $root.Values.global.terraform -}}
- name: terraform-init
  image: "{{ include "radius.terraform.image" $root }}"
  args:
  {{- if $tf.downloadUrl }}
  - "--url={{ $tf.downloadUrl }}"
  {{- if $tf.downloadChecksum }}
  - "--sha256={{ $tf.downloadChecksum }}"
  {{- end }}
  {{- else }}
  - "--version={{ required "global.terraform.version is required when global.terraform.downloadUrl is empty" $tf.version }}"
  {{- end }}
  # Must match the global path in pkg/recipes/terraform/install.go.
  - "--dir=/terraform/.terraform-global"
  - "--legacy-dir={{ .path }}"
  {{- if $root.Values.global.rootCA.cert }}
  env:
  - name: {{ $root.Values.global.rootCA.sslCertDirEnvVar }}
    value: {{ $root.Values.global.rootCA.mountPath }}
  {{- end }}
  terminationMessagePolicy: FallbackToLogsOnError
  volumeMounts:
  - name: terraform
    mountPath: {{ .path }}
  # Scratch space for the download, since the root filesystem is read-only.
  - name: terraform-init-tmp
    mountPath: /tmp
  {{- if $root.Values.global.rootCA.cert }}
  - name: {{ $root.Values.global.rootCA.volumeName }}
    mountPath: {{ $root.Values.global.rootCA.mountPath }}
    readOnly: true
  {{- end }}
  {{- if $root.Values.global.appendRootCA.cert }}
  - name: ssl-certs
    mountPath: /etc/ssl/certs
    readOnly: true
  {{- end }}
  securityContext:
    allowPrivilegeEscalation: false
    capabilities:
      drop:
        - ALL
    readOnlyRootFilesystem: true
    runAsNonRoot: true
    runAsUser: 65532
    runAsGroup: 65532
    seccompProfile:
      type: RuntimeDefault
{{- end -}}

{{/*
radius.terraform.env renders resource provider environment variables that control the runtime
Terraform download fallback. With the default release download, the runtime is pinned to the same
version as the init container. With global.terraform.downloadUrl, the runtime must only use the
binary installed by the init container, so it never silently installs a different Terraform build.
*/}}
{{- define "radius.terraform.env" -}}
{{- $tf := .Values.global.terraform -}}
{{- if eq $tf.enabled true -}}
{{- if $tf.downloadUrl -}}
- name: RADIUS_TERRAFORM_PREMOUNTED_ONLY
  value: "true"
{{- else if $tf.version -}}
- name: RADIUS_TERRAFORM_VERSION
  value: {{ $tf.version | quote }}
{{- end -}}
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

{{/*
Returns the Pod Security Admission level (baseline or restricted) from global.rbac.podSecurityLevel.
An empty value defaults to baseline. Fails rendering for any other value.
*/}}
{{- define "radius.authz.podSecurityLevel" -}}
{{- $rbac := .Values.global.rbac | default dict -}}
{{- $level := $rbac.podSecurityLevel | default "baseline" -}}
{{- if not (has $level (list "baseline" "restricted")) -}}
{{- fail (printf "global.rbac.podSecurityLevel must be \"baseline\" or \"restricted\", got %q" $level) -}}
{{- end -}}
{{- $level -}}
{{- end -}}

{{/*
Control-plane service accounts and Secrets in the release namespace, as JSON lists that are
also valid CEL list literals. Used by the admission policy and, for the Secrets, the
applications-rp configuration (authorization.controlPlaneSecrets); keep in sync with the
serviceaccount.yaml templates and the Secrets the chart creates.
*/}}
{{- define "radius.authz.controlPlaneServiceAccounts" -}}
{{- list "ucp" "applications-rp" "dynamic-rp" "bicep-de" "controller" "dashboard" "database" "radius-api-readiness" "pre-upgrade" | toJson -}}
{{- end -}}

{{- define "radius.authz.controlPlaneSecrets" -}}
{{- list "database-secret" "radius-encryption-key" "radius-aws-fingerprint-key" "ucp-cert" .Values.global.rootCA.secretName .Values.global.targetCluster.secretName | uniq | toJson -}}
{{- end -}}

{{/*
Usernames whose requests the admission policy validates, as a JSON list.
TODO(D7): default to the application-template identity once the broker issues it.
*/}}
{{- define "radius.authz.applicationIdentities" -}}
{{- $admission := (.Values.global.rbac | default dict).admission | default dict -}}
{{- $identities := $admission.applicationIdentities | default (list (printf "system:serviceaccount:%s:applications-rp" .Release.Namespace)) -}}
{{- $identities | toJson -}}
{{- end -}}

{{/*
Path where a component's mTLS Secret (<component>-mtls) is mounted.
*/}}
{{- define "radius.authz.mtls.mountPath" -}}
/var/run/radius/mtls
{{- end -}}

{{/*
Renders the authorization config block for a component. Renders nothing when
the authorization mode is off, so a default install is unchanged.
*/}}
{{- define "radius.authz.serverConfig" -}}
{{- $mode := include "radius.authz.mode" . -}}
{{- if ne $mode "off" }}
authorization:
  mode: {{ $mode | quote }}
  certificateDirectory: {{ include "radius.authz.mtls.mountPath" . | quote }}
  namespace: {{ .Release.Namespace | quote }}
{{- end }}
{{- end -}}

{{/*
Renders the ucp connection block for a component that calls UCP. When the authorization
mode is not off, the component calls UCP's internal mTLS port directly instead of going
through the Kubernetes API server.
*/}}
{{- define "radius.authz.ucpConnection" -}}
{{- if ne (include "radius.authz.mode" .) "off" }}
ucp:
  kind: direct
  direct:
    endpoint: "https://ucp.{{ .Release.Namespace }}.svc:9444/apis/api.ucp.dev/v1alpha3"
{{- else }}
ucp:
  kind: kubernetes
{{- end }}
{{- end -}}

{{/*
Renders the mTLS volume mount for a component container when the authorization mode is not off.
*/}}
{{- define "radius.authz.mtls.volumeMount" -}}
{{- if ne (include "radius.authz.mode" .) "off" }}
- name: mtls
  mountPath: {{ include "radius.authz.mtls.mountPath" . }}
  readOnly: true
{{- end }}
{{- end -}}

{{/*
Renders the mTLS Secret volume for a component when the authorization mode is not off.
Call with (dict "root" $ "component" "<component>").
*/}}
{{- define "radius.authz.mtls.volume" -}}
{{- if ne (include "radius.authz.mode" .root) "off" }}
- name: mtls
  secret:
    secretName: {{ .component }}-mtls
{{- end }}
{{- end -}}
