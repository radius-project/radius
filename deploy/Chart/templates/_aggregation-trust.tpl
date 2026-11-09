{{/*
Read the APIService before its public journal. Together with the resourceVersion
in the rendered APIService, this prevents a stale Helm render from overwriting a
later Radius trust update. The journal is not a Helm-owned resource.
*/}}
{{- define "radius.aggregationTrust" -}}
{{- $intent := include "radius.bootstrapState" . | mustFromJson -}}
{{- if and $intent (eq $intent.phase "preparing") -}}
{{- fail "Aggregation bootstrap must verify its selected CA leaf before rendering public trust." -}}
{{- end -}}
{{- $api := lookup "apiregistration.k8s.io/v1" "APIService" "" "v1alpha3.api.ucp.dev" -}}
{{- $annotations := dig "metadata" "annotations" (dict) $api -}}
{{- range $key := list "cert-manager.io/inject-ca-from" "cert-manager.io/inject-ca-from-secret" "cert-manager.io/inject-apiserver-ca" -}}
{{- if hasKey $annotations $key -}}
{{- fail "Radius APIService has CA injection configured. A safe explicit handover is required; removing the annotation alone is not sufficient." -}}
{{- end -}}
{{- end -}}
{{- range $field := dig "metadata" "managedFields" (list) $api -}}
{{- if eq $field.manager "cert-manager-cainjector" -}}
{{- fail "Radius APIService has injector write history. Establish a safe explicit handover before Radius manages its trust." -}}
{{- end -}}
{{- end -}}
{{- $journal := lookup "v1" "ConfigMap" .Release.Namespace "radius-ca-rotation" -}}
{{- $result := dict "api" $api -}}
{{- if $journal -}}
{{- $state := include "radius.rotationJournal" (dict "journal" $journal "root" .) | mustFromJson -}}
{{- if ne $state.apiServiceUID (dig "metadata" "uid" "" $api) -}}
{{- fail "Radius CA rotation journal does not match this installation." -}}
{{- end -}}
{{- if or (ne (index $annotations "meta.helm.sh/release-name") .Release.Name) (ne (index $annotations "meta.helm.sh/release-namespace") .Release.Namespace) (dig "spec" "insecureSkipTLSVerify" false $api) (ne (dig "spec" "service" "name" "" $api) "ucp") (ne (dig "spec" "service" "namespace" "" $api) .Release.Namespace) -}}
{{- fail "Radius APIService ownership or verified service reference changed during managed trust." -}}
{{- end -}}
{{- if or (ne (index $annotations "radius.io/trust-owner") .Release.Name) (ne (index $annotations "radius.io/trust-journal-uid") $journal.metadata.uid) (ne (index $annotations "radius.io/trust-revision") (printf "%s/complete" $state.id)) (not $state.newCA) (ne (dig "spec" "caBundle" "" $api) $state.newCA) -}}
{{- fail "Radius APIService trust differs from its completed rotation journal. Recover the transaction instead of overwriting trust." -}}
{{- end -}}
{{- $_ := set $result "caBundle" $state.newCA -}}
{{- $_ := set $result "bootstrap" (or (default false $state.bootstrap) (and $intent (eq $intent.phase "complete"))) -}}
{{- $_ := set $result "annotations" (dict "radius.io/trust-owner" .Release.Name "radius.io/trust-journal-uid" $journal.metadata.uid "radius.io/trust-revision" (printf "%s/complete" $state.id)) -}}
{{- else if or (hasKey $annotations "radius.io/trust-journal-uid") (hasKey $annotations "radius.io/trust-revision") (hasKey $annotations "radius.io/trust-owner") -}}
{{- fail "Radius APIService has managed trust but its rotation journal is missing. Recover the journal before upgrading." -}}
{{- else -}}
{{- $bootstrap := include "radius.bootstrapPresentation" . | mustFromJson -}}
{{- if $bootstrap -}}
{{- if and $api (ne (dig "metadata" "annotations" "radius.io/aggregation-bootstrap" "" $api) $bootstrap.id) -}}
{{- fail "Fresh aggregation bootstrap cannot adopt an existing APIService." -}}
{{- end -}}
{{- $_ := set $result "bootstrap" true -}}
{{- $_ := set $result "caBundle" $bootstrap.newCA -}}
{{- $_ := set $result "annotations" (dict "radius.io/aggregation-bootstrap" $bootstrap.id) -}}
{{- else if and (not $api) .Values.global (ne (include "radius.authz.mode" .) "off") -}}
{{- fail "Fresh enabled Radius requires verified configured-CA bootstrap; use the matching rad install workflow." -}}
{{- end -}}
{{- end -}}
{{- toJson $result -}}
{{- end -}}
