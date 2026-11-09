{{/*
The supported operator-managed approver-policy v0.29.0 profile is deliberately
bounded. Unknown layouts fail closed rather than pretending to evaluate arbitrary
approvers, CEL programs, or authorization webhooks. All reads use live lookup;
offline rendering cannot supply prerequisite evidence through values.
*/}}
{{- define "radius.authz.certificateComponents" -}}
{{- $mtls := (.Values.global.rbac.mtls | default dict) -}}
{{- if eq (default "radius" $mtls.provider) "external" -}}
{{- list "ucp" | toJson -}}
{{- else -}}
{{- list "ucp" "applications-rp" "dynamic-rp" "controller" "bicep-de" | toJson -}}
{{- end -}}
{{- end -}}

{{- define "radius.authz.policySpec" -}}
{{- $san := printf "%s.%s.svc" .component .namespace -}}
{{- $rule := printf "cr.username == '%s' && cr.namespace == '%s' && cr.name.startsWith('%s-cert-')" .requestor .namespace .component -}}
{{- dict
  "selector" (dict "issuerRef" .issuer "namespace" (dict "matchNames" (list .namespace)))
  "allowed" (dict "dnsNames" (dict "values" (list $san) "required" true "validations" (list (dict "rule" $rule)))
    "isCA" false "usages" (list "digital signature" "server auth" "client auth"))
  "constraints" (dict "minDuration" "24h" "maxDuration" "24h"
    "privateKey" (dict "algorithm" "ECDSA" "minSize" 256 "maxSize" 256)) | toJson -}}
{{- end -}}

{{- define "radius.authz.policyReady" -}}
{{- $ready := false -}}
{{- range .status.conditions | default list -}}
{{- if and (eq .type "Ready") (eq .status "True") (eq (int64 .observedGeneration) (int64 $.metadata.generation)) -}}
{{- $ready = true -}}
{{- end -}}
{{- end -}}
{{- if or (not $ready) .metadata.deletionTimestamp -}}
{{- fail (printf "issuerRef prerequisite: %s %s must be Ready at its current generation." .kind .metadata.name) -}}
{{- end -}}
{{- end -}}

{{/*
Verify a controller's rollout and every live pod, not just a Deployment object
or a stale Available condition. The supported image is operator-selected, but its
official binary, version, command and configuration layout must be inspectable.
*/}}
{{- define "radius.authz.policyController" -}}
{{- $ref := .ref -}}
{{- if or (empty $ref.namespace) (empty $ref.deployment) -}}
{{- fail (printf "issuerRef prerequisite: requestPolicy.%s requires namespace and deployment." .role) -}}
{{- end -}}
{{- $d := lookup "apps/v1" "Deployment" $ref.namespace $ref.deployment -}}
{{- if not $d -}}
{{- fail (printf "issuerRef prerequisite: %s Deployment %s/%s is missing; use a live cluster-connected Helm render." .role $ref.namespace $ref.deployment) -}}
{{- end -}}
{{- $replicas := int64 $d.spec.replicas -}}
{{- if or (lt $replicas 1) (ne (int64 $d.status.observedGeneration) (int64 $d.metadata.generation)) (ne (int64 $d.status.updatedReplicas) $replicas) (ne (int64 $d.status.readyReplicas) $replicas) (ne (int64 $d.status.replicas) $replicas) $d.metadata.deletionTimestamp -}}
{{- fail (printf "issuerRef prerequisite: %s Deployment must have a complete current Ready rollout." .role) -}}
{{- end -}}
{{- $spec := $d.spec.template.spec -}}
{{- if or (empty $spec.serviceAccountName) (ne (len $spec.containers) 1) (not (empty $spec.initContainers)) (empty $d.spec.selector.matchLabels) (not (empty $d.spec.selector.matchExpressions)) -}}
{{- fail (printf "issuerRef prerequisite: %s requires an explicit ServiceAccount and one unmodified controller container." .role) -}}
{{- end -}}
{{- $container := first $spec.containers -}}
{{- if or (not (empty $container.command)) (not (empty $container.envFrom)) -}}
{{- fail (printf "issuerRef prerequisite: %s command/environment overrides are unsupported." .role) -}}
{{- end -}}
{{- range $container.env | default list -}}
{{- if or (ne $.role "certManager") (ne .name "POD_NAMESPACE") .value (ne .valueFrom.fieldRef.fieldPath "metadata.namespace") -}}
{{- fail (printf "issuerRef prerequisite: %s environment overrides are unsupported except the cert-manager POD_NAMESPACE downward API." $.role) -}}
{{- end -}}
{{- end -}}
{{- if eq .role "certManager" -}}
{{- if not (regexMatch `(^|/)cert-manager-controller:v1\.(2[0-9]|[3-9][0-9])\.[0-9]+$` $container.image) -}}
{{- fail "issuerRef prerequisite: certManager requires an inspectable official cert-manager-controller v1.20+ image tag." -}}
{{- end -}}
{{- $disabled := false -}}
{{- range $container.args | default list -}}
{{- if hasPrefix "--controllers" . -}}
{{- if or $disabled (not (has . (list "--controllers=-certificaterequests-approver" "--controllers=*,-certificaterequests-approver"))) -}}
{{- fail "issuerRef prerequisite: certManager must explicitly disable only certificaterequests-approver, without duplicate controller flags." -}}
{{- end -}}
{{- $disabled = true -}}
{{- else if not (regexMatch `^--(v|cluster-resource-namespace|leader-election-namespace|acme-http01-solver-image|max-concurrent-challenges|cluster-issuer-ambient-credentials|issuer-ambient-credentials|enable-certificate-owner-ref|dns01-recursive-nameservers|dns01-recursive-nameservers-only|logging-format|metrics-listen-address|healthz-listen-address)=` .) -}}
{{- fail (printf "issuerRef prerequisite: unsupported certManager argument %q; config files and approval/controller overrides are not accepted." .) -}}
{{- end -}}
{{- end -}}
{{- if not $disabled -}}
{{- fail "issuerRef prerequisite: certManager default auto-approval must be explicitly disabled with --controllers=-certificaterequests-approver." -}}
{{- end -}}
{{- else -}}
{{- if not (regexMatch `(^|/)cert-manager-approver-policy:v0\.29\.0$` $container.image) -}}
{{- fail "issuerRef prerequisite: approverPolicy requires the supported cert-manager-approver-policy:v0.29.0 controller." -}}
{{- end -}}
{{- range $container.args | default list -}}
{{- if not (regexMatch `^--(log-format|log-level|metrics-bind-address|readiness-probe-bind-address|webhook-host|webhook-port|webhook-service-name|webhook-ca-secret-namespace|webhook-ca-secret-name)=` .) -}}
{{- fail (printf "issuerRef prerequisite: unsupported approverPolicy argument %q." .) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $count := 0 -}}
{{- range (lookup "v1" "Pod" $ref.namespace "").items | default list -}}
{{- $matches := true -}}
{{- $pod := . -}}
{{- range $key, $value := $d.spec.selector.matchLabels -}}
{{- if ne (index ($pod.metadata.labels | default dict) $key) $value -}}{{- $matches = false -}}{{- end -}}
{{- end -}}
{{- if and (eq .spec.serviceAccountName $spec.serviceAccountName) (not $matches) -}}
{{- fail (printf "issuerRef prerequisite: unexpected pod uses the %s controller ServiceAccount." $.role) -}}
{{- end -}}
{{- if $matches -}}
{{- $ready := false -}}
{{- range .status.conditions | default list -}}
{{- if and (eq .type "Ready") (eq .status "True") -}}{{- $ready = true -}}{{- end -}}
{{- end -}}
{{- $same := and (eq (len .spec.containers) 1) (empty .spec.initContainers) -}}
{{- if $same -}}
{{- $actual := first .spec.containers -}}
{{- range $field := list "image" "args" "command" "env" "envFrom" -}}
{{- if ne (toJson (index $actual $field)) (toJson (index $container $field)) -}}{{- $same = false -}}{{- end -}}
{{- end -}}
{{- end -}}
{{- if or (not $ready) .metadata.deletionTimestamp (ne .status.phase "Running") (ne .spec.serviceAccountName $spec.serviceAccountName) (not $same) -}}
{{- fail (printf "issuerRef prerequisite: every %s pod must be Running/Ready with the verified controller configuration." $.role) -}}
{{- end -}}
{{- $count = add1 $count -}}
{{- end -}}
{{- end -}}
{{- if ne (int64 $count) $replicas -}}
{{- fail (printf "issuerRef prerequisite: %s Ready pod inventory does not match the Deployment." .role) -}}
{{- end -}}
{{- dict "name" $spec.serviceAccountName "namespace" $ref.namespace | toJson -}}
{{- end -}}

{{- define "radius.authz.policySubjectMatches" -}}
{{- $sa := .sa -}}
{{- $s := .subject -}}
{{- $username := printf "system:serviceaccount:%s:%s" $sa.namespace $sa.name -}}
{{- if or
  (and (eq $s.kind "ServiceAccount") (eq $s.name $sa.name) (eq $s.namespace $sa.namespace))
  (and (eq $s.kind "User") (eq $s.name $username))
  (and (eq $s.kind "Group") (has $s.name (list "system:authenticated" "system:serviceaccounts" (printf "system:serviceaccounts:%s" $sa.namespace)))) -}}
true
{{- end -}}
{{- end -}}

{{- define "radius.authz.policyRuleMatches" -}}
{{- if and
  (or (has .group .rule.apiGroups) (has "*" .rule.apiGroups))
  (or (has .resource .rule.resources) (has "*" .rule.resources))
  (or (has .verb .rule.verbs) (has "*" .rule.verbs))
  (or (empty .rule.resourceNames) (has .name .rule.resourceNames)) -}}
true
{{- end -}}
{{- end -}}

{{/*
The cert-manager webhook protects immutable requestor identity and authorizes
Approved/Denied writes. A disabled or namespace-excluded webhook defeats the
policy even when both controllers and their RBAC are otherwise correct.
*/}}
{{- define "radius.authz.policyNamespaceSelected" -}}
{{- $namespace := lookup "v1" "Namespace" "" .namespace -}}
{{- if not $namespace -}}{{- fail "issuerRef prerequisite: create the Radius namespace before verifying admission selectors." -}}{{- end -}}
{{- $labels := $namespace.metadata.labels | default dict -}}
{{- $selected := true -}}
{{- range $key, $value := .selector.matchLabels | default dict -}}
{{- if ne (index $labels $key) $value -}}{{- $selected = false -}}{{- end -}}
{{- end -}}
{{- range .selector.matchExpressions | default list -}}
{{- $exists := hasKey $labels .key -}}
{{- $matches := has (index $labels .key) (.values | default list) -}}
{{- if eq .operator "In" -}}{{- if not (and $exists $matches) -}}{{- $selected = false -}}{{- end -}}
{{- else if eq .operator "NotIn" -}}{{- if and $exists $matches -}}{{- $selected = false -}}{{- end -}}
{{- else if eq .operator "Exists" -}}{{- if not $exists -}}{{- $selected = false -}}{{- end -}}
{{- else if eq .operator "DoesNotExist" -}}{{- if $exists -}}{{- $selected = false -}}{{- end -}}
{{- else -}}{{- fail "issuerRef prerequisite: unsupported admission namespace selector operator." -}}
{{- end -}}
{{- end -}}
{{- if $selected -}}true{{- end -}}
{{- end -}}

{{- define "radius.authz.policyWebhook" -}}
{{- $ref := .Values.global.rbac.certificates.requestPolicy.webhook | default dict -}}
{{- if or (empty $ref.namespace) (empty $ref.service) (empty $ref.configuration) -}}
{{- fail "issuerRef prerequisite: requestPolicy.webhook requires namespace, service and configuration." -}}
{{- end -}}
{{- range $kind := list "MutatingWebhookConfiguration" "ValidatingWebhookConfiguration" -}}
{{- $config := lookup "admissionregistration.k8s.io/v1" $kind "" $ref.configuration -}}
{{- $found := false -}}
{{- range $config.webhooks | default list -}}
{{- if eq .name "webhook.cert-manager.io" -}}
{{- $found = true -}}
{{- $path := "/validate" -}}{{- if eq $kind "MutatingWebhookConfiguration" -}}{{- $path = "/mutate" -}}{{- end -}}
{{- $namespaceSelected := include "radius.authz.policyNamespaceSelected" (dict "selector" (.namespaceSelector | default dict) "namespace" $.Release.Namespace) -}}
{{- if or (ne .failurePolicy "Fail") (not $namespaceSelected) (not (empty .objectSelector)) (not (empty .matchConditions)) (empty .clientConfig.caBundle) (ne .clientConfig.service.namespace $ref.namespace) (ne .clientConfig.service.name $ref.service) (ne .clientConfig.service.path $path) .clientConfig.url -}}
{{- fail "issuerRef prerequisite: cert-manager admission webhooks must fail closed for the Radius namespace and all request objects using the configured service and CA." -}}
{{- end -}}
{{- $rules := false -}}
{{- range .rules -}}
{{- if and (has "cert-manager.io" .apiGroups) (has "v1" .apiVersions) (has "CREATE" .operations) (or (empty .scope) (has .scope (list "*" "Namespaced"))) -}}
{{- if and (eq $kind "MutatingWebhookConfiguration") (has "certificaterequests" .resources) -}}{{- $rules = true -}}{{- end -}}
{{- if and (eq $kind "ValidatingWebhookConfiguration") (has "UPDATE" .operations) (has "*/*" .resources) -}}{{- $rules = true -}}{{- end -}}
{{- end -}}
{{- end -}}
{{- if not $rules -}}{{- fail "issuerRef prerequisite: cert-manager admission rules must protect CertificateRequest creation and status updates." -}}{{- end -}}
{{- end -}}
{{- end -}}
{{- if not $found -}}{{- fail (printf "issuerRef prerequisite: cert-manager %s is missing." $kind) -}}{{- end -}}
{{- end -}}
{{- $endpoints := lookup "v1" "Endpoints" $ref.namespace $ref.service -}}
{{- $ready := 0 -}}
{{- range $endpoints.subsets | default list -}}
{{- range .addresses | default list -}}
{{- if or (ne .targetRef.kind "Pod") (ne .targetRef.namespace $ref.namespace) (empty .targetRef.name) -}}
{{- fail "issuerRef prerequisite: cert-manager webhook endpoints must reference local Ready Pods." -}}
{{- end -}}
{{- $pod := lookup "v1" "Pod" $ref.namespace .targetRef.name -}}
{{- if or (empty .ip) (ne .ip $pod.status.podIP) -}}{{- fail "issuerRef prerequisite: cert-manager webhook endpoint address must match its Ready Pod IP." -}}{{- end -}}
{{- $condition := false -}}
{{- range $pod.status.conditions | default list -}}{{- if and (eq .type "Ready") (eq .status "True") -}}{{- $condition = true -}}{{- end -}}{{- end -}}
{{- if or (not $condition) (ne $pod.status.phase "Running") $pod.metadata.deletionTimestamp (ne (len $pod.spec.containers) 1) -}}
{{- fail "issuerRef prerequisite: cert-manager webhook endpoint Pods must be Running/Ready." -}}
{{- end -}}
{{- $container := first $pod.spec.containers -}}
{{- if or (not (regexMatch `(^|/)cert-manager-webhook:v1\.(2[0-9]|[3-9][0-9])\.[0-9]+$` $container.image)) (not (empty $container.command)) (not (empty $container.envFrom)) (not (empty $pod.spec.initContainers)) -}}
{{- fail "issuerRef prerequisite: admission endpoints must run the official cert-manager-webhook v1.20+ binary." -}}
{{- end -}}
{{- range $container.env | default list -}}
{{- if or (ne .name "POD_NAMESPACE") .value (ne .valueFrom.fieldRef.fieldPath "metadata.namespace") -}}{{- fail "issuerRef prerequisite: unsupported cert-manager webhook environment override." -}}{{- end -}}
{{- end -}}
{{- range $container.args | default list -}}
{{- if not (regexMatch `^--(v|secure-port|dynamic-serving-ca-secret-namespace|dynamic-serving-ca-secret-name|dynamic-serving-dns-names|metrics-listen-address|healthz-port|logging-format|tls-min-version|tls-cipher-suites)=` .) -}}
{{- fail (printf "issuerRef prerequisite: unsupported cert-manager webhook argument %q." .) -}}
{{- end -}}
{{- end -}}
{{- $ready = add1 $ready -}}
{{- end -}}
{{- end -}}
{{- if eq (int64 $ready) 0 -}}{{- fail "issuerRef prerequisite: cert-manager admission webhook has no Ready endpoints." -}}{{- end -}}
{{- end -}}

{{- define "radius.authz.issuerPolicy.validate" -}}
{{- $root := . -}}
{{- $c := .Values.global.rbac.certificates -}}
{{- $issuer := $c.issuerRef | default dict -}}
{{- if or (ne $issuer.group "cert-manager.io") (not (has $issuer.kind (list "Issuer" "ClusterIssuer"))) (not (kindIs "string" $issuer.name)) (not (regexMatch `^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$` (toString $issuer.name))) -}}
{{- fail "issuerRef requires issuerRef.group=cert-manager.io, kind=Issuer or ClusterIssuer, and an existing issuer name." -}}
{{- end -}}
{{- $issuerNamespace := "" -}}
{{- if eq $issuer.kind "Issuer" -}}{{- $issuerNamespace = .Release.Namespace -}}{{- end -}}
{{- $existing := lookup "cert-manager.io/v1" $issuer.kind $issuerNamespace $issuer.name -}}
{{- if not $existing -}}{{- fail "issuerRef prerequisite: the configured existing Issuer/ClusterIssuer is missing; use a live cluster-connected Helm render." -}}{{- end -}}
{{- include "radius.authz.policyReady" $existing -}}
{{- $prereqs := $c.requestPolicy | default dict -}}
{{- $cm := include "radius.authz.policyController" (dict "ref" ($prereqs.certManager | default dict) "role" "certManager") | fromJson -}}
{{- $ap := include "radius.authz.policyController" (dict "ref" ($prereqs.approverPolicy | default dict) "role" "approverPolicy") | fromJson -}}
{{- include "radius.authz.policyWebhook" . -}}
{{- if eq (toJson $cm) (toJson $ap) -}}{{- fail "issuerRef prerequisite: certManager and approverPolicy must use separate ServiceAccounts." -}}{{- end -}}
{{- $requestor := printf "system:serviceaccount:%s:%s" $cm.namespace $cm.name -}}
{{- $expected := dict -}}
{{- range list "ucp" "applications-rp" "dynamic-rp" "controller" "bicep-de" -}}
{{- $_ := set $expected (printf "radius-%s-%s" $root.Release.Namespace .) (include "radius.authz.policySpec" (dict "component" . "namespace" $root.Release.Namespace "issuer" $issuer "requestor" $requestor) | fromJson) -}}
{{- end -}}
{{- $policies := (lookup "policy.cert-manager.io/v1alpha1" "CertificateRequestPolicy" "" "").items | default list -}}
{{- $found := dict -}}
{{- range $policies -}}
{{- $selector := .spec.selector | default dict -}}
{{- $overlaps := true -}}
{{- range $key := list "name" "kind" "group" -}}
{{- $pattern := index ($selector.issuerRef | default dict) $key | default "*" -}}
{{- if not (regexMatch (printf "^%s$" (replace "\\*" ".*" (regexQuoteMeta $pattern))) (index $issuer $key)) -}}{{- $overlaps = false -}}{{- end -}}
{{- end -}}
{{- if $overlaps -}}
{{- $spec := deepCopy .spec -}}
{{- range $field := list "minDuration" "maxDuration" -}}
{{- if eq (index ($spec.constraints | default dict) $field) "24h0m0s" -}}{{- $_ := set $spec.constraints $field "24h" -}}{{- end -}}
{{- end -}}
{{- if or (not (hasKey $expected .metadata.name)) (ne (toJson $spec) (toJson (index $expected .metadata.name))) -}}
{{- fail (printf "issuerRef prerequisite: overlapping policy %s is not an exact supported Radius policy; policies are OR-ed, so additional grants can bypass a restrictive policy." .metadata.name) -}}
{{- end -}}
{{- include "radius.authz.policyReady" . -}}
{{- $_ := set $found .metadata.name true -}}
{{- end -}}
{{- end -}}
{{- range include "radius.authz.certificateComponents" . | fromJsonArray -}}
{{- if not (hasKey $found (printf "radius-%s-%s" $root.Release.Namespace .)) -}}{{- fail "issuerRef prerequisite: install all expected Ready Radius CertificateRequestPolicies before installing Radius." -}}{{- end -}}
{{- end -}}
{{- $bindings := concat ((lookup "rbac.authorization.k8s.io/v1" "ClusterRoleBinding" "" "").items | default list) ((lookup "rbac.authorization.k8s.io/v1" "RoleBinding" .Release.Namespace "").items | default list) -}}
{{- $usable := dict -}}
{{- $approverPermissions := dict -}}
{{- range $binding := $bindings -}}
{{- $roleNamespace := "" -}}
{{- if eq $binding.roleRef.kind "Role" -}}{{- $roleNamespace = $binding.metadata.namespace -}}{{- end -}}
{{- $role := lookup "rbac.authorization.k8s.io/v1" $binding.roleRef.kind $roleNamespace $binding.roleRef.name -}}
{{- if not $role -}}{{- fail (printf "issuerRef prerequisite: binding %s references a missing role." $binding.metadata.name) -}}{{- end -}}
{{- range $subject := $binding.subjects | default list -}}
{{- $isCM := include "radius.authz.policySubjectMatches" (dict "sa" $cm "subject" $subject) -}}
{{- $isAP := include "radius.authz.policySubjectMatches" (dict "sa" $ap "subject" $subject) -}}
{{- if and (eq $binding.roleRef.name "cluster-admin") (or $isCM $isAP) -}}
{{- fail "issuerRef prerequisite: certificate controllers must not have cluster-admin authority." -}}
{{- end -}}
{{- range $rule := $role.rules | default list -}}
{{- if ne $binding.roleRef.name "cluster-admin" -}}
{{- range $component := list "ucp" "applications-rp" "dynamic-rp" "controller" "bicep-de" -}}
{{- $isComponent := include "radius.authz.policySubjectMatches" (dict "sa" (dict "name" $component "namespace" $root.Release.Namespace) "subject" $subject) -}}
{{- range $resource := list "certificates" "certificaterequests" "issuers" "clusterissuers" -}}
{{- range $verb := list "create" "update" "patch" -}}
{{- $name := "" -}}{{- if $rule.resourceNames -}}{{- $name = first $rule.resourceNames -}}{{- end -}}
{{- if and $isComponent (include "radius.authz.policyRuleMatches" (dict "rule" $rule "group" "cert-manager.io" "resource" $resource "verb" $verb "name" $name)) -}}
{{- fail (printf "issuerRef prerequisite: component %s must not create or modify certificate or issuer resources." $component) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- range $name, $_ := $expected -}}
{{- if hasKey $found $name -}}
{{- if include "radius.authz.policyRuleMatches" (dict "rule" $rule "group" "policy.cert-manager.io" "resource" "certificaterequestpolicies" "verb" "use" "name" $name) -}}
{{- if $isCM -}}{{- $_ := set $usable $name true -}}{{- end -}}
{{- if and (ne $binding.roleRef.name "cluster-admin") (not (or (and (eq $subject.kind "ServiceAccount") (eq $subject.name $cm.name) (eq $subject.namespace $cm.namespace)) (and (eq $subject.kind "User") (eq $subject.name $requestor)))) -}}
{{- fail (printf "issuerRef prerequisite: binding %s grants Radius policy use to a subject other than the cert-manager requestor." $binding.metadata.name) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- $signerName := $issuer.name -}}
{{- if eq $issuer.kind "Issuer" -}}{{- $signerName = printf "%s.%s" $root.Release.Namespace $issuer.name -}}{{- end -}}
{{- $signer := printf "%ss.cert-manager.io/%s" (lower $issuer.kind) $signerName -}}
{{- $canApprove := include "radius.authz.policyRuleMatches" (dict "rule" $rule "group" "cert-manager.io" "resource" "signers" "verb" "approve" "name" $signer) -}}
{{- if and (eq $binding.kind "ClusterRoleBinding") (or $canApprove (and (or (has "cert-manager.io" $rule.apiGroups) (has "*" $rule.apiGroups)) (or (has "signers" $rule.resources) (has "*" $rule.resources)) (or (has "approve" $rule.verbs) (has "*" $rule.verbs)) (or (has (printf "%ss.cert-manager.io/*" (lower $issuer.kind)) $rule.resourceNames) (has (printf "%ss.cert-manager.io/%s" (lower $issuer.kind) $issuer.name) $rule.resourceNames)))) -}}
{{- if and (ne $binding.roleRef.name "cluster-admin") (not (and (eq $subject.kind "ServiceAccount") (eq $subject.name $ap.name) (eq $subject.namespace $ap.namespace))) -}}
{{- fail (printf "issuerRef prerequisite: binding %s permits another subject (including the default approver) to approve the selected signer." $binding.metadata.name) -}}
{{- end -}}
{{- if $isAP -}}{{- $_ := set $approverPermissions "signer" true -}}{{- end -}}
{{- end -}}
{{- if and $isAP (eq $binding.kind "ClusterRoleBinding") -}}
{{- range $permission := list
  (dict "group" "cert-manager.io" "resource" "certificaterequests/status" "verb" "patch")
  (dict "group" "cert-manager.io" "resource" "certificaterequests" "verb" "list")
  (dict "group" "cert-manager.io" "resource" "certificaterequests" "verb" "watch")
  (dict "group" "policy.cert-manager.io" "resource" "certificaterequestpolicies" "verb" "list")
  (dict "group" "policy.cert-manager.io" "resource" "certificaterequestpolicies" "verb" "watch")
  (dict "group" "policy.cert-manager.io" "resource" "certificaterequestpolicies/status" "verb" "patch")
  (dict "group" "rbac.authorization.k8s.io" "resource" "roles" "verb" "list")
  (dict "group" "rbac.authorization.k8s.io" "resource" "roles" "verb" "watch")
  (dict "group" "rbac.authorization.k8s.io" "resource" "rolebindings" "verb" "list")
  (dict "group" "rbac.authorization.k8s.io" "resource" "rolebindings" "verb" "watch")
  (dict "group" "rbac.authorization.k8s.io" "resource" "clusterroles" "verb" "list")
  (dict "group" "rbac.authorization.k8s.io" "resource" "clusterroles" "verb" "watch")
  (dict "group" "rbac.authorization.k8s.io" "resource" "clusterrolebindings" "verb" "list")
  (dict "group" "rbac.authorization.k8s.io" "resource" "clusterrolebindings" "verb" "watch")
  (dict "group" "" "resource" "namespaces" "verb" "list")
  (dict "group" "" "resource" "namespaces" "verb" "watch")
  (dict "group" "authorization.k8s.io" "resource" "subjectaccessreviews" "verb" "create") -}}
{{- if include "radius.authz.policyRuleMatches" (merge (dict "rule" $rule "name" "") $permission) -}}
{{- $_ := set $approverPermissions (printf "%s/%s" $permission.resource $permission.verb) true -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- range $name, $_ := $found -}}
{{- if not (hasKey $usable $name) -}}{{- fail "issuerRef prerequisite: the actual cert-manager controller ServiceAccount lacks policy use RBAC in the Radius namespace." -}}{{- end -}}
{{- end -}}
{{- if ne (len $approverPermissions) 18 -}}{{- fail "issuerRef prerequisite: approver-policy lacks required cluster-wide signer approval, request/policy status, informer read/watch, or SubjectAccessReview permissions." -}}{{- end -}}
{{- end -}}
