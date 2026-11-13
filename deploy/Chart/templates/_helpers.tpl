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
Returns the authorization mode (off, dryRun, or enforce) derived from global.rbac.
Fails rendering when dry run is combined with enforcement.
*/}}
{{- define "radius.authz.mode" -}}
{{- $rbac := .Values.global.rbac | default dict -}}
{{- range $setting := list "startupProof" "aggregationProof" -}}
{{- $legacy := get $rbac $setting -}}
{{- if $legacy -}}
{{- if or (not (kindIs "map" $legacy)) (get $legacy "enabled") (get $legacy "expectedTrustSHA256") -}}
{{- fail "legacy certificate startup proof configuration is unsupported; authenticated lifecycle observation is not available" -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $_ := include "radius.authz.tls.provider" . -}}
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

{{/* Reject retired or misspelled provider settings even when authorization is off. */}}
{{- define "radius.authz.tls.provider" -}}
{{- $rbac := .Values.global.rbac | default dict -}}
{{- if hasKey $rbac "tlsProvider" -}}
{{- fail "global.rbac.tlsProvider is not supported; use global.rbac.tls.provider" -}}
{{- end -}}
{{- if hasKey $rbac "mtls" -}}
{{- fail "global.rbac.mtls is retired; migrate explicitly to global.rbac.tls.provider" -}}
{{- end -}}
{{- $tls := $rbac.tls | default dict -}}
{{- $provider := "radius" -}}
{{- if hasKey $tls "provider" -}}
{{- $provider = $tls.provider -}}
{{- end -}}
{{- if not (has $provider (list "radius" "external")) -}}
{{- fail "global.rbac.tls.provider must be radius or external" -}}
{{- end -}}
{{- $provider -}}
{{- end -}}

{{/* Explicit compatible-engine gate; retired enabled settings must never become Off. */}}
{{- define "radius.de.internalAuth.enabled" -}}
{{- $legacy := .Values.de.ucpMTLS | default dict -}}
{{- if $legacy.enabled -}}
{{- fail "de.ucpMTLS.enabled is retired; select a token-capable engine image and migrate explicitly to de.internalAuth.enabled" -}}
{{- end -}}
{{- $settings := .Values.de.internalAuth | default dict -}}
{{- $enabled := false -}}
{{- if hasKey $settings "enabled" -}}
{{- $enabled = $settings.enabled -}}
{{- end -}}
{{- if not (kindIs "bool" $enabled) -}}
{{- fail "de.internalAuth.enabled must be a boolean" -}}
{{- end -}}
{{- if and $enabled (eq (include "radius.authz.mode" .) "off") -}}
{{- fail "de.internalAuth.enabled requires global.rbac.dryRun=true or global.rbac.enabled=true" -}}
{{- end -}}
{{- $enabled -}}
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
{{- $secrets := list "database-secret" "radius-encryption-key" "radius-aws-fingerprint-key" "ucp-cert" "ucp-apiserver-tls" "radius-ca" "radius-apiserver-ca" "radius-apiserver-trust" .Values.global.rootCA.secretName .Values.global.targetCluster.secretName -}}
{{- if ne (include "radius.authz.mode" .) "off" -}}
{{- $c := .Values.global.rbac.certificates | default dict -}}
{{- if and (eq (include "radius.authz.certificates.mode" .) "caSecret") $c.caSecret -}}{{- $secrets = append $secrets $c.caSecret -}}{{- end -}}
{{- end -}}
{{- $secrets | uniq | toJson -}}
{{- end -}}

{{/*
Name of the Secret holding UCP's aggregated API server certificate. When the authorization mode is
not off, cert-manager issues it from radius-apiserver-issuer into ucp-apiserver-tls; otherwise the
chart generates the ucp-cert Secret.
*/}}
{{- define "radius.ucp.apiServerSecretName" -}}
{{- if ne (include "radius.authz.mode" .) "off" -}}
ucp-apiserver-tls
{{- else -}}
ucp-cert
{{- end -}}
{{- end -}}

{{/*
Hash of the authorization settings a component reads at startup, for the checksum/authz-config pod
annotation, so changing any of them rolls the pods.
*/}}
{{- define "radius.authz.configChecksum" -}}
{{- $mode := include "radius.authz.mode" . -}}
{{- $config := $mode -}}
{{- if ne $mode "off" -}}
{{- $config = list $mode (include "radius.authz.podSecurityLevel" .) (include "radius.authz.identityConfig" .) (include "radius.authz.spiffe.csiDriverName" .) (include "radius.authz.controlPlaneSecrets" .) (include "radius.de.internalAuth.enabled" .) | join "\n" -}}
{{- $aggregation := include "radius.authz.certificates.publicSource" (dict "root" . "plane" "aggregation") | fromJson -}}
{{- if $aggregation.bundle -}}
{{- $config = printf "%s\n%s" $config $aggregation.digest -}}
{{- end -}}
{{- if eq (include "radius.authz.tls.provider" .) "radius" -}}
{{- $internal := include "radius.authz.certificates.publicSource" (dict "root" . "plane" "internal") | fromJson -}}
{{- if $internal.bundle -}}
{{- $config = printf "%s\n%s" $config $internal.digest -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $config | sha256sum -}}
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
Usernames of the cert-manager controller that may request certificates from the Radius bootstrap and
API server issuers. Empty defaults to cert-manager's default installation; rad install/upgrade sets the
ServiceAccount of the cert-manager it installs or reuses.
*/}}
{{- define "radius.authz.certManagerRequesters" -}}
{{- $certManager := (.Values.global.rbac | default dict).certManager | default dict -}}
{{- $certManager.requesters | default (list "system:serviceaccount:cert-manager:cert-manager") | toJson -}}
{{- end -}}

{{/*
SPIFFE trust domain of the Radius server certificates.
*/}}
{{- define "radius.authz.spiffe.trustDomain" -}}
{{- $spiffe := (.Values.global.rbac | default dict).spiffe | default dict -}}
{{- $spiffe.trustDomain | default "radius.internal" -}}
{{- end -}}

{{/*
Renders the authorization settings shared by every component's config: where the SPIFFE server
certificate, the Radius CA bundle, and the projected ServiceAccount tokens are mounted, the trust
domain, and the release namespace.
*/}}
{{- define "radius.authz.identityConfig" -}}
tlsProvider: {{ include "radius.authz.tls.provider" . | quote }}
certificateDirectory: {{ include "radius.authz.spiffe.mountPath" . | quote }}
caDirectory: {{ include "radius.authz.caBundle.mountPath" . | quote }}
tokenDirectory: {{ include "radius.authz.tokens.mountPath" . | quote }}
trustDomain: {{ include "radius.authz.spiffe.trustDomain" . | quote }}
namespace: {{ .Release.Namespace | quote }}
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
  {{- include "radius.authz.identityConfig" . | nindent 2 }}
{{- end }}
{{- end -}}

{{/*
Renders the ucp connection block for a component that calls UCP. When the authorization
mode is not off, the component calls UCP's internal port directly instead of going
through the Kubernetes API server.
*/}}
{{- define "radius.authz.ucpConnection" -}}
{{- if ne (include "radius.authz.mode" .) "off" }}
ucp:
  kind: direct
  direct:
    endpoint: "{{ if eq (include "radius.authz.tls.provider" .) "radius" }}https{{ else }}http{{ end }}://ucp.{{ .Release.Namespace }}.svc:9444/apis/api.ucp.dev/v1alpha3"
{{- else }}
ucp:
  kind: kubernetes
{{- end }}
{{- end -}}

{{/*
Path where every component mounts the Radius CA bundle (ca.crt). csi-driver-spiffe writes only
tls.crt and tls.key because Radius does not configure it with a source CA bundle, which it must read
at startup, before this chart has created radius-ca.
*/}}
{{- define "radius.authz.caBundle.mountPath" -}}
/var/run/radius/ca
{{- end -}}

{{/*
Renders the Radius CA bundle volume mount for a component when the authorization mode is not off.
Servers chain their SPIFFE certificate to it and clients verify servers against it. Kubelet updates
the file when cert-manager renews radius-ca, and components reload it.
*/}}
{{- define "radius.authz.caBundle.volumeMount" -}}
{{- if and (ne (include "radius.authz.mode" .) "off") (eq (include "radius.authz.tls.provider" .) "radius") }}
- name: radius-ca
  mountPath: {{ include "radius.authz.caBundle.mountPath" . }}
  readOnly: true
{{- end }}
{{- end -}}

{{/*
Renders the Radius CA bundle volume. Only ca.crt is projected; the CA private key in the same
Secret is not mounted. The Secret is required, so pods wait until cert-manager has issued radius-ca.
*/}}
{{- define "radius.authz.caBundle.volume" -}}
{{- if and (ne (include "radius.authz.mode" .) "off") (eq (include "radius.authz.tls.provider" .) "radius") }}
{{- $source := include "radius.authz.certificates.publicSource" (dict "root" . "plane" "internal") | fromJson }}
- name: radius-ca
  {{- if eq $source.kind "ConfigMap" }}
  configMap:
    name: {{ $source.name }}
  {{- else }}
  secret:
    secretName: {{ $source.name }}
  {{- end }}
    items:
      - key: ca.crt
        path: ca.crt
{{- end }}
{{- end -}}

{{/*
Path where a receiver's csi-driver-spiffe server certificate (tls.crt, tls.key) is mounted.
*/}}
{{- define "radius.authz.spiffe.mountPath" -}}
/var/run/radius/spiffe
{{- end -}}

{{/*
Name of the CSIDriver that Radius's csi-driver-spiffe instance registers.
*/}}
{{- define "radius.authz.spiffe.csiDriverName" -}}
{{- $spiffe := (.Values.global.rbac | default dict).spiffe | default dict -}}
{{- $spiffe.csiDriverName | default "spiffe.csi.radapp.io" -}}
{{- end -}}

{{/*
Renders the csi-driver-spiffe server certificate volume mount for a receiver container when the
authorization mode is not off.
*/}}
{{- define "radius.authz.spiffe.volumeMount" -}}
{{- if and (ne (include "radius.authz.mode" .) "off") (eq (include "radius.authz.tls.provider" .) "radius") }}
- name: spiffe
  mountPath: {{ include "radius.authz.spiffe.mountPath" . }}
  readOnly: true
{{- end }}
{{- end -}}

{{/*
Renders the csi-driver-spiffe server certificate volume for a receiver when the authorization mode
is not off. The driver requests the certificate with the pod's ServiceAccount token, so the
certificate carries the SPIFFE ID spiffe://<trust domain>/ns/<namespace>/sa/<service account>.
Files are group-readable by gid 65532, the user Radius containers run as.
*/}}
{{- define "radius.authz.spiffe.volume" -}}
{{- if and (ne (include "radius.authz.mode" .) "off") (eq (include "radius.authz.tls.provider" .) "radius") }}
- name: spiffe
  csi:
    driver: {{ include "radius.authz.spiffe.csiDriverName" . }}
    readOnly: true
    volumeAttributes:
      spiffe.csi.cert-manager.io/fs-group: "65532"
{{- end }}
{{- end -}}

{{/*
Directory under which a caller's projected ServiceAccount tokens are mounted, one subdirectory per
receiver: <mountPath>/<receiver>/token.
*/}}
{{- define "radius.authz.tokens.mountPath" -}}
/var/run/radius/tokens
{{- end -}}

{{/*
Renders the projected ServiceAccount token volume mounts for a caller container when the
authorization mode is not off. Call with (dict "root" $ "receivers" (list "<receiver>" ...)).
*/}}
{{- define "radius.authz.tokens.volumeMounts" -}}
{{- if ne (include "radius.authz.mode" .root) "off" }}
{{- range .receivers }}
- name: token-{{ . }}
  mountPath: {{ include "radius.authz.tokens.mountPath" $.root }}/{{ . }}
  readOnly: true
{{- end }}
{{- end }}
{{- end -}}

{{/*
Renders one projected ServiceAccount token volume per receiver a caller calls, when the
authorization mode is not off. Each token's audience is the receiver's name, which the receiver's
token verifier requires; the kubelet refreshes the 10-minute token before it expires.
Call with (dict "root" $ "receivers" (list "<receiver>" ...)).
*/}}
{{- define "radius.authz.tokens.volumes" -}}
{{- if ne (include "radius.authz.mode" .root) "off" }}
{{- range .receivers }}
- name: token-{{ . }}
  projected:
    sources:
      - serviceAccountToken:
          audience: {{ . | quote }}
          expirationSeconds: 600
          path: token
{{- end }}
{{- end }}
{{- end -}}
