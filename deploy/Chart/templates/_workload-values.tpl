{{- define "radius.validateNetworkPolicyValues" -}}
{{- if and .Values.networkPolicies.enabled (not .Values.networkPolicies.controlPlaneCIDRs) -}}
{{- fail "networkPolicies.enabled=true requires networkPolicies.controlPlaneCIDRs to be set to the cluster's node/control-plane CIDR(s) so the Kubernetes API server can reach UCP on :9443. Set networkPolicies.controlPlaneCIDRs, or set networkPolicies.enabled=false to install without these policies." -}}
{{- end -}}
{{- end -}}

{{- define "radius.validateTerraformPath" -}}
{{- if ne (index .root.Values .component).terraform.path "/terraform" -}}
{{- fail (printf "%s.terraform.path must be \"/terraform\" -- the runtime configmap (deploy/Chart/templates/%s/configmaps.yaml) and pkg/recipes/terraform/install.go both hard-code /terraform, so overriding the mount path produces an inconsistent deployment and silently breaks Terraform execution/caching." .component .directory) -}}
{{- end -}}
{{- end -}}

{{- define "radius.validateBuildKitValues" -}}
{{- if .Values.dynamicrp.buildkit.enabled -}}
{{- $maxParallelism := .Values.dynamicrp.buildkit.maxParallelism -}}
{{- $maxParallelismError := "dynamicrp.buildkit.maxParallelism must be an integer between 1 and 2147483647" -}}
{{- if not (or (kindIs "int" $maxParallelism) (kindIs "int64" $maxParallelism) (kindIs "float64" $maxParallelism)) -}}
{{- fail $maxParallelismError -}}
{{- end -}}
{{- $maxParallelismFloat := float64 $maxParallelism -}}
{{- if or (lt $maxParallelismFloat 1.0) (gt $maxParallelismFloat 2147483647.0) (ne $maxParallelismFloat (floor $maxParallelismFloat)) -}}
{{- fail $maxParallelismError -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "radius.validateExternalWorkloadValues" -}}
{{- $_ := include "radius.authz.podSecurityLevel" . -}}
{{- include "radius.validateNetworkPolicyValues" . -}}
{{- include "radius.validateTerraformPath" (dict "root" . "component" "rp" "directory" "rp") -}}
{{- include "radius.validateTerraformPath" (dict "root" . "component" "dynamicrp" "directory" "dynamic-rp") -}}
{{- include "radius.validateBuildKitValues" . -}}
{{- end -}}
