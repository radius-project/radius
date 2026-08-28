#!/bin/bash

# Hermetic tests for the routes Gateway lifecycle action.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly SCRIPT="${SCRIPT_DIR}/routes-gateway.sh"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
readonly REPO_ROOT
readonly AZURE_WORKFLOW="${REPO_ROOT}/.github/extension/run-rad-commands-azure.yml"
readonly AWS_WORKFLOW="${REPO_ROOT}/.github/extension/run-rad-commands-aws.yml"

TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT
readonly BIN="${TEST_ROOT}/bin"
readonly STATE="${TEST_ROOT}/state"
readonly CALLS="${TEST_ROOT}/calls.log"
readonly OUTPUT="${TEST_ROOT}/output.log"
readonly APP_FILE="${TEST_ROOT}/app.bicep"
readonly COMPILED="${TEST_ROOT}/compiled.json"
readonly BICEP="${BIN}/bicep"

ACTION_EXIT=0

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

write_fakes() {
    mkdir -p "${BIN}" "${STATE}"

    cat >"${BICEP}" <<'EOF'
#!/bin/bash
set -euo pipefail
cp "${COMPILED}" "$4"
EOF

    cat >"${BIN}/rad" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'rad %s\n' "$*" >>"${CALLS}"
case "${1:-} ${2:-}" in
    "env show")
        printf '%s\n' "${ENV_JSON}"
        ;;
    "recipe-pack show")
        if [[ -n "${PACK_JSON_OVERRIDE}" ]]; then
            printf '%s\n' "${PACK_JSON_OVERRIDE}"
        else
            jq -nc --arg source "${PACK_SOURCE}" \
                '{properties:{recipes:{"Radius.Compute/routes":{kind:"bicep",source:$source}}}}'
        fi
        ;;
    "app list")
        if [[ "${RAD_DISCOVERY_FAIL}" == "app" ]]; then
            exit 1
        fi
        printf '%s\n' "${APP_LIST_JSON}"
        ;;
    "resource list")
        if [[ "${RAD_DISCOVERY_FAIL}" == "resource" ]]; then
            exit 1
        fi
        app=""
        while (($#)); do
            if [[ "$1" == "--application" ]]; then
                app="$2"
                break
            fi
            shift
        done
        if [[ -n "${RESOURCE_JSON_OVERRIDE}" ]]; then
            printf '%s\n' "${RESOURCE_JSON_OVERRIDE}"
        elif [[ ",${ROUTES_APPS}," == *",${app},"* ]]; then
            printf '%s\n' \
                '[{"type":"Radius.Compute/routes@2025-08-01-preview"}]'
        else
            printf '%s\n' '[]'
        fi
        ;;
    *)
        echo "unexpected rad command: $*" >&2
        exit 91
        ;;
esac
EOF

    cat >"${BIN}/curl" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'curl %s\n' "$*" >>"${CALLS}"
out=""
while (($#)); do
    if [[ "$1" == "-o" ]]; then
        out="$2"
        break
    fi
    shift
done
: >"${out}"
EOF

    cat >"${BIN}/helm" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'helm %s\n' "$*" >>"${CALLS}"
args=" $* "
case "${args}" in
    *" status contour "*)
        if [[ "${HELM_STATUS_WARNING}" == "true" ]]; then
            echo 'WARNING: Kubernetes configuration file is group-readable' >&2
        fi
        if [[ -f "${STATE}/helm-installed" ]]; then
            printf '%s\n' '{"name":"contour","namespace":"radius-system"}'
            exit 0
        fi
        case "${HELM_STATE}" in
            absent)
                echo 'Error: release: not found' >&2
                exit 1
                ;;
            failure)
                echo 'synthetic Helm connectivity failure' >&2
                exit 1
                ;;
            malformed)
                printf '%s\n' '{"name":"wrong"}'
                ;;
            present)
                printf '%s\n' \
                    '{"name":"contour","namespace":"radius-system"}'
                ;;
        esac
        ;;
    *" get values contour "*)
        if [[ "${HELM_VALUES_STATE}" == "failure" ]]; then
            echo 'synthetic Helm values failure' >&2
            exit 1
        elif [[ "${HELM_VALUES_STATE}" == "malformed" ]]; then
            printf '%s\n' 'not-json'
        elif [[ "${HELM_OWNED}" == "true" ||
            -f "${STATE}/helm-installed" ]]; then
            printf '%s\n' \
                '{"commonAnnotations":{"radius-project.io/routes-gateway-lifecycle":"v1"}}'
        else
            printf '%s\n' '{}'
        fi
        ;;
    *" upgrade --install contour "*)
        [[ "${HELM_FAIL}" != "true" ]] || exit 1
        touch "${STATE}/helm-installed"
        if [[ "${args}" == *"envoy.service.type=LoadBalancer"* ]]; then
            printf 'LoadBalancer' >"${STATE}/service-type"
        else
            printf 'ClusterIP' >"${STATE}/service-type"
        fi
        ;;
    *" uninstall contour "*)
        touch "${STATE}/helm-uninstalled"
        ;;
    *)
        echo "unexpected helm command: $*" >&2
        exit 92
        ;;
esac
EOF

    cat >"${BIN}/kubectl" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'kubectl %s\n' "$*" >>"${CALLS}"

if [[ "${1:-}" == "--kubeconfig" ]]; then
    shift 2
fi

if [[ "${1:-}" == "create" && "${2:-}" == "--dry-run=client" ]]; then
    printf '%s\n' "${CRD_LIST_JSON}"
    exit 0
fi

if [[ "${1:-}" == "create" && "${2:-}" == "-f" ]]; then
    body="$(cat)"
    printf 'create-body %s\n' \
        "$(tr '\n' ' ' <<<"${body}")" >>"${CALLS}"
    if jq -e . >/dev/null 2>&1 <<<"${body}"; then
        name="$(jq -r '.metadata.name' <<<"${body}")"
        touch "${STATE}/crd-${name}"
    elif grep -q '^kind: Namespace' <<<"${body}"; then
        touch "${STATE}/namespace"
    elif grep -q '^kind: GatewayClass' <<<"${body}"; then
        touch "${STATE}/gatewayclass"
    elif grep -q '^kind: Gateway' <<<"${body}"; then
        touch "${STATE}/gateway"
    fi
    exit 0
fi

if [[ "${1:-}" == "wait" ]]; then
    if [[ -n "${WAIT_FAIL}" && "$*" == *"${WAIT_FAIL}"* ]]; then
        exit 1
    fi
    exit 0
fi

if [[ "${1:-}" == "delete" ]]; then
    exit 0
fi

[[ "${1:-}" == "get" ]] || {
    echo "unexpected kubectl command: $*" >&2
    exit 93
}

resource="${2:-}"
case "${resource}" in
    customresourcedefinition/*)
        name="${resource#*/}"
        if [[ -f "${STATE}/crd-${name}" ||
            "${CRDS_STATE}" == "complete" ]]; then
            printf 'customresourcedefinition.apiextensions.k8s.io/%s\n' \
                "${name}"
            exit 0
        fi
        case "${CRDS_STATE}" in
            missing)
                echo "Error from server (NotFound): customresourcedefinitions.apiextensions.k8s.io \"${name}\" not found" >&2
                exit 1
                ;;
            failure)
                echo 'synthetic CRD connectivity failure' >&2
                exit 1
                ;;
            malformed)
                printf '%s\n' 'unexpected-crd-output'
                exit 0
                ;;
        esac
        if [[ "$*" == *"jsonpath="* ]]; then
            [[ "${CRDS_OWNED}" == "true" ]] && printf 'v1'
        fi
        exit 0
        ;;
    customresourcedefinition)
        name="${3:-}"
        if [[ "${name}" == "httpproxies.projectcontour.io" ||
            "${name}" == "tlscertificatedelegations.projectcontour.io" ]]; then
            case "${CONTOUR_CRDS_STATE}" in
                present)
                    printf '%s\n' '{"kind":"CustomResourceDefinition"}'
                    exit 0
                    ;;
                missing)
                    echo "Error from server (NotFound): customresourcedefinitions.apiextensions.k8s.io \"${name}\" not found" >&2
                    exit 1
                    ;;
                failure)
                    echo "synthetic Contour CRD discovery failure" >&2
                    exit 1
                    ;;
            esac
        fi
        if [[ "$*" == *"jsonpath="* ]]; then
            [[ "${CRDS_OWNED}" == "true" ]] && printf 'v1'
        fi
        exit 0
        ;;
    namespace)
        [[ "${NAMESPACE_STATE}" == "present" ||
            -f "${STATE}/namespace" ]]
        ;;
    deployment)
        count=0
        if [[ "${COMPONENTS_STATE}" == "complete" ||
            -f "${STATE}/helm-installed" ]]; then
            count=1
        fi
        jq -nc --argjson count "${count}" \
            '{items:[range(0;$count)|{metadata:{name:"contour"}}]}'
        ;;
    service)
        count=0
        if [[ "${COMPONENTS_STATE}" == "complete" ||
            -f "${STATE}/helm-installed" ]]; then
            count=1
        fi
        type="${SERVICE_TYPE}"
        [[ -f "${STATE}/service-type" ]] &&
            type="$(cat "${STATE}/service-type")"
        ingress='[]'
        [[ "${type}" == "LoadBalancer" ]] &&
            ingress='[{"hostname":"example.test"}]'
        jq -nc --argjson count "${count}" --arg type "${type}" \
            --argjson ingress "${ingress}" \
            '{items:[range(0;$count)|{
              metadata:{name:"contour-envoy"},
              spec:{type:$type},
              status:{loadBalancer:{ingress:$ingress}}
            }]}'
        ;;
    endpoints)
        if [[ "${ENDPOINTS_READY}" == "true" ]]; then
            printf '%s\n' '{"subsets":[{"addresses":[{"ip":"10.0.0.1"}]}]}'
        else
            printf '%s\n' '{"subsets":[]}'
        fi
        ;;
    gatewayclass)
        if [[ "${3:-}" == "contour" ]]; then
            if [[ "${GATEWAYCLASS_STATE}" == "present" ||
                -f "${STATE}/gatewayclass" ]]; then
                if [[ "$*" == *"routes-gateway-lifecycle"* ]]; then
                    [[ "${GATEWAY_RESOURCES_OWNED}" == "true" ]] &&
                        printf 'v1'
                elif [[ "$*" == *"jsonpath="* ]]; then
                    printf '%s' "${GATEWAYCLASS_CONTROLLER}"
                fi
                exit 0
            fi
            exit 1
        fi
        if [[ "$*" == *"jsonpath="* ]]; then
            printf '%s' "${BYO_CLASS_CONTROLLER}"
            exit 0
        fi
        [[ "${BYO_STATE}" == "complete" ]]
        ;;
    gateway)
        if [[ "${3:-}" == "radius" ]]; then
            if [[ "${GATEWAY_STATE}" == "present" ||
                -f "${STATE}/gateway" ]]; then
                if [[ "$*" == *"routes-gateway-lifecycle"* ]]; then
                    [[ "${GATEWAY_RESOURCES_OWNED}" == "true" ]] &&
                        printf 'v1'
                elif [[ "$*" == *"-o json"* ]]; then
                    if [[ "${GATEWAY_LISTENERS_VALID}" == "true" ]]; then
                        jq -nc '{
                          spec:{
                            gatewayClassName:"contour",
                            listeners:[
                              {
                                name:"http",protocol:"HTTP",port:80,
                                allowedRoutes:{namespaces:{from:"All"}}
                              },
                              {
                                name:"tls",protocol:"TLS",port:443,
                                tls:{mode:"Passthrough"},
                                allowedRoutes:{namespaces:{from:"All"}}
                              }
                            ]
                          }
                        }'
                    else
                        printf '%s\n' \
                            '{"spec":{"gatewayClassName":"contour","listeners":[]}}'
                    fi
                fi
                exit 0
            fi
            exit 1
        fi
        if [[ "$*" == *"-o json"* ]]; then
            printf '%s\n' \
                '{"spec":{"gatewayClassName":"byo-class","listeners":[]}}'
            exit 0
        fi
        [[ "${BYO_STATE}" == "complete" ]]
        ;;
    gatewayclasses.gateway.networking.k8s.io | \
    gateways.gateway.networking.k8s.io | \
    httproutes.gateway.networking.k8s.io | \
    backendtlspolicies.gateway.networking.k8s.io | \
    grpcroutes.gateway.networking.k8s.io | \
    tcproutes.gateway.networking.k8s.io | \
    tlsroutes.gateway.networking.k8s.io | \
    udproutes.gateway.networking.k8s.io | \
    referencegrants.gateway.networking.k8s.io)
        if [[ -n "${CLUSTER_JSON_OVERRIDE}" ]]; then
            printf '%s\n' "${CLUSTER_JSON_OVERRIDE}"
        elif [[ "${SHARED_OBJECTS}" == "true" &&
            "${resource}" == "${SHARED_OBJECT_RESOURCE}" ]]; then
            printf '%s\n' '{"items":[{"metadata":{"annotations":{}}}]}'
        else
            printf '%s\n' '{"items":[]}'
        fi
        ;;
    ingresses.networking.k8s.io)
        if [[ "${SHARED_INGRESS}" == "true" ]]; then
            printf '%s\n' '{"items":[{"metadata":{"name":"shared-ingress"}}]}'
        else
            printf '%s\n' '{"items":[]}'
        fi
        ;;
    httpproxies.projectcontour.io | \
    tlscertificatedelegations.projectcontour.io)
        if [[ "${CONTOUR_SHARED_OBJECTS}" == "true" &&
            "${resource}" == "${CONTOUR_OBJECT_RESOURCE}" ]]; then
            printf '%s\n' '{"items":[{"metadata":{"name":"shared-contour"}}]}'
        else
            printf '%s\n' '{"items":[]}'
        fi
        ;;
    *)
        echo "unexpected kubectl get: $*" >&2
        exit 94
        ;;
esac
EOF

    chmod +x "${BIN}"/*
}

reset_case() {
    rm -rf "${STATE}"
    mkdir -p "${STATE}"
    : >"${CALLS}"
    : >"${OUTPUT}"
    touch "${APP_FILE}"
    jq -nc '{
      resources:{
        app:{
          type:"Radius.Compute/routes@2025-08-01-preview",
          existing:false
        }
      }
    }' >"${COMPILED}"

    ENV_JSON="$(
        jq -nc \
            '{properties:{recipePacks:["/scope/providers/Radius.Core/recipePacks/default"]}}'
    )"
    PACK_SOURCE='ghcr.io/radius-project/kube-recipes/routes:test'
    PACK_JSON_OVERRIDE=''
    APP_LIST_JSON='[]'
    ROUTES_APPS=''
    RESOURCE_JSON_OVERRIDE=''
    RAD_DISCOVERY_FAIL=''
    CRDS_STATE='complete'
    CRDS_OWNED='false'
    NAMESPACE_STATE='present'
    COMPONENTS_STATE='complete'
    HELM_STATE='absent'
    HELM_OWNED='false'
    HELM_VALUES_STATE='valid'
    HELM_STATUS_WARNING='false'
    HELM_FAIL='false'
    SERVICE_TYPE='ClusterIP'
    ENDPOINTS_READY='true'
    GATEWAYCLASS_STATE='present'
    GATEWAYCLASS_CONTROLLER='projectcontour.io/gateway-controller'
    GATEWAY_STATE='present'
    GATEWAY_RESOURCES_OWNED='false'
    GATEWAY_LISTENERS_VALID='true'
    BYO_STATE='complete'
    BYO_CLASS_CONTROLLER='example.io/controller'
    SHARED_OBJECTS='false'
    SHARED_OBJECT_RESOURCE='httproutes.gateway.networking.k8s.io'
    CLUSTER_JSON_OVERRIDE=''
    SHARED_INGRESS='false'
    CONTOUR_CRDS_STATE='missing'
    CONTOUR_SHARED_OBJECTS='false'
    CONTOUR_OBJECT_RESOURCE='httpproxies.projectcontour.io'
    WAIT_FAIL=''
    TARGET_KUBECONFIG_OVERRIDE=''
    CRD_LIST_JSON="$(
        jq -nc '{
          apiVersion:"v1",
          kind:"List",
          items:[
            "gatewayclasses","gateways","httproutes","backendtlspolicies",
            "referencegrants",
            "grpcroutes","tcproutes","tlsroutes","udproutes"
          ] | map({
            apiVersion:"apiextensions.k8s.io/v1",
            kind:"CustomResourceDefinition",
            metadata:{name:(. + ".gateway.networking.k8s.io")}
          })
        }'
    )"
    export ENV_JSON PACK_SOURCE PACK_JSON_OVERRIDE APP_LIST_JSON ROUTES_APPS
    export RESOURCE_JSON_OVERRIDE
    export RAD_DISCOVERY_FAIL CRDS_STATE CRDS_OWNED NAMESPACE_STATE
    export COMPONENTS_STATE HELM_STATE HELM_OWNED HELM_VALUES_STATE
    export HELM_STATUS_WARNING
    export HELM_FAIL SERVICE_TYPE
    export ENDPOINTS_READY GATEWAYCLASS_STATE GATEWAYCLASS_CONTROLLER
    export GATEWAY_STATE GATEWAY_RESOURCES_OWNED GATEWAY_LISTENERS_VALID
    export BYO_STATE
    export BYO_CLASS_CONTROLLER SHARED_OBJECTS SHARED_OBJECT_RESOURCE
    export CLUSTER_JSON_OVERRIDE
    export SHARED_INGRESS CONTOUR_CRDS_STATE CONTOUR_SHARED_OBJECTS
    export CONTOUR_OBJECT_RESOURCE
    export WAIT_FAIL CRD_LIST_JSON TARGET_KUBECONFIG_OVERRIDE
}

run_action() {
    local mode="${1:-ensure}"
    local exposure="${2:-}"
    local explicit="${3:-false}"
    local gateway_name="${4-}"
    local gateway_namespace="${5-}"
    set +e
    (
        export PATH="${BIN}:${PATH}" CALLS STATE COMPILED
        export MODE="${mode}" EXPOSURE="${exposure}"
        export GATEWAY_EXPLICIT="${explicit}"
        export GATEWAY_NAME="${gateway_name}"
        export GATEWAY_NAMESPACE="${gateway_namespace}"
        export ENVIRONMENT_NAME="test" APP_FILE
        export BICEP_BIN="${BICEP}" RADIUS_ROUTES_WAIT_TIMEOUT="1s"
        export RADIUS_ROUTES_RETRY_DELAY="0"
        export TARGET_KUBECONFIG="${TARGET_KUBECONFIG_OVERRIDE}"
        bash "${SCRIPT}"
    ) >"${OUTPUT}" 2>&1
    ACTION_EXIT=$?
    set -e
}

assert_success() {
    ((ACTION_EXIT == 0)) ||
        fail "$1: expected success, got ${ACTION_EXIT}
$(cat "${OUTPUT}")"
}

assert_failure() {
    ((ACTION_EXIT != 0)) ||
        fail "$1: expected failure
$(cat "${OUTPUT}")"
}

assert_output() {
    grep -qF -- "$1" "${OUTPUT}" ||
        fail "expected output '$1'
$(cat "${OUTPUT}")"
}

assert_call() {
    grep -qF -- "$1" "${CALLS}" ||
        fail "expected call '$1'
$(cat "${CALLS}")"
}

assert_no_call() {
    if grep -qF -- "$1" "${CALLS}"; then
        fail "unexpected call '$1'
$(cat "${CALLS}")"
    fi
}

write_fakes

# No routes and custom routes recipes are strict no-ops.
reset_case
jq -nc '{resources:{container:{type:"Radius.Compute/containers"}}}' \
    >"${COMPILED}"
run_action
assert_success "app without routes"
assert_output "declares no non-existing Radius.Compute/routes"
assert_no_call "kubectl"

# Gateway-specific variables are irrelevant to no-route applications.
run_action ensure invalid true byo ""
assert_success "no-route ignores Gateway variables"
assert_output "declares no non-existing Radius.Compute/routes"
assert_no_call "rad env show"
assert_no_call "kubectl"

reset_case
jq -nc '{
  resources:{
    custom:{
      type:"Example.Resources/widgets",
      properties:{
        template:{
          resources:[
            {
              type:"Radius.Compute/routes@2025-08-01-preview",
              existing:false
            },
            "arbitrary template content"
          ]
        }
      }
    }
  }
}' >"${COMPILED}"
run_action
assert_success "non-deployment template property"
assert_output "declares no non-existing Radius.Compute/routes"
assert_no_call "rad env show"
assert_no_call "kubectl"

reset_case
jq -nc '{
  resources:{
    module:{
      type:"Microsoft.Resources/deployments",
      properties:{
        template:{
          resources:[
            {
              type:"Radius.Compute/routes@2025-08-01-preview",
              existing:false
            }
          ]
        }
      }
    }
  }
}' >"${COMPILED}"
run_action
assert_success "nested module routes"
assert_call "rad env show test --preview -o json"

reset_case
jq -nc '{
  resources:[
    {
      type:"Radius.Compute/routes@2025-08-01-preview",
      existing:false
    },
    {
      type:"Microsoft.Resources/deployments",
      properties:{template:{resources:["invalid"]}}
    }
  ]
}' >"${COMPILED}"
run_action
assert_failure "route before malformed nested resource"
assert_output "compiled application template has invalid resources data"
assert_no_call "rad env show"
assert_no_call "kubectl"

reset_case
jq -nc '{
  resources:{
    module:{
      type:"Microsoft.Resources/deployments",
      properties:{template:{resources:"invalid"}}
    }
  }
}' >"${COMPILED}"
run_action
assert_failure "malformed nested resources"
assert_output "compiled application template has invalid resources data"
assert_no_call "rad env show"
assert_no_call "kubectl"

reset_case
jq -nc '{resources:"invalid"}' >"${COMPILED}"
run_action
assert_failure "malformed compiled resources"
assert_output "compiled application template has invalid resources data"
assert_no_call "rad env show"
assert_no_call "kubectl"

reset_case
PACK_SOURCE='br:example.test/custom/routes:v1'
export PACK_SOURCE
run_action ensure invalid true byo ""
assert_success "custom routes recipe"
assert_output "Custom routes recipe detected"
assert_no_call "kubectl"

reset_case
PACK_JSON_OVERRIDE="$(jq -nc '{properties:{recipes:[]}}')"
export PACK_JSON_OVERRIDE
run_action ensure
assert_failure "malformed recipe discovery"
assert_output "returned invalid recipe data"
assert_no_call "kubectl"

# Invalid and conflicting inputs fail before any cluster mutation.
reset_case
run_action ensure internal
assert_failure "invalid exposure"
assert_output "must be 'private' or 'public'"
assert_no_call "kubectl"

reset_case
run_action ensure public true byo byo-system
assert_failure "BYO exposure conflict"
assert_output "cannot be set with an explicit BYO Gateway"
assert_no_call "kubectl"

reset_case
run_action ensure "" true byo ""
assert_failure "BYO missing namespace"
assert_output "RADIUS_ROUTES_GATEWAY_NAMESPACE is required"
assert_no_call "kubectl"

# BYO is validation-only and reports incomplete dependencies.
reset_case
run_action ensure "" true byo byo-system
assert_success "complete BYO"
assert_output "BYO Gateway byo-system/byo is ready"
assert_no_call "kubectl create"
assert_no_call "helm "

reset_case
CRDS_STATE='missing'
export CRDS_STATE
run_action ensure "" true byo byo-system
assert_failure "incomplete BYO CRDs"
assert_output "Gateway API CRDs are incomplete"
assert_no_call "kubectl create"

# Private is the default and never requests a LoadBalancer.
reset_case
run_action ensure
assert_success "private default"
assert_output "ClusterIP exposure"
assert_call "rad recipe-pack show /scope/providers/Radius.Core/recipePacks/default -o json"
assert_no_call "envoy.service.type=LoadBalancer"

# A configured target kubeconfig must exist and be readable.
reset_case
TARGET_KUBECONFIG_OVERRIDE="${TEST_ROOT}/missing-kubeconfig"
export TARGET_KUBECONFIG_OVERRIDE
run_action ensure
assert_failure "missing target kubeconfig"
assert_output "target kubeconfig is not a readable regular file"
assert_no_call "--kubeconfig ${TARGET_KUBECONFIG_OVERRIDE}"
assert_no_call "kubectl create"

reset_case
TARGET_KUBECONFIG_OVERRIDE="${TEST_ROOT}/kubeconfig-directory"
mkdir -p "${TARGET_KUBECONFIG_OVERRIDE}"
export TARGET_KUBECONFIG_OVERRIDE
run_action ensure
assert_failure "directory target kubeconfig"
assert_output "target kubeconfig is not a readable regular file"
assert_no_call "--kubeconfig ${TARGET_KUBECONFIG_OVERRIDE}"
assert_no_call "kubectl create"

# Public warns with every affected application and validates LoadBalancer.
reset_case
APP_LIST_JSON="$(jq -nc '[{name:"alpha"},{name:"beta"}]')"
ROUTES_APPS='alpha,beta'
SERVICE_TYPE='LoadBalancer'
export APP_LIST_JSON ROUTES_APPS SERVICE_TYPE
run_action ensure public
assert_success "public exposure"
assert_output "PUBLIC ROUTES EXPOSURE"
assert_output "alpha, beta"

reset_case
HELM_STATE='present'
HELM_OWNED='true'
SERVICE_TYPE='ClusterIP'
export HELM_STATE HELM_OWNED SERVICE_TYPE
run_action ensure public
assert_success "private to public"
assert_call "envoy.service.type=LoadBalancer"
assert_call "envoy.service.externalTrafficPolicy=Local"

reset_case
RAD_DISCOVERY_FAIL='app'
export RAD_DISCOVERY_FAIL
run_action ensure public
assert_failure "public warning discovery failure"
assert_output "failed to identify applications affected by public exposure"
assert_no_call "kubectl create"
assert_no_call "helm upgrade"

# A managed release is upgraded from public back to private.
reset_case
HELM_STATE='present'
HELM_OWNED='true'
SERVICE_TYPE='LoadBalancer'
export HELM_STATE HELM_OWNED SERVICE_TYPE
run_action ensure private
assert_success "public to private"
assert_call "envoy.service.type=ClusterIP"
assert_call "envoy.service.externalTrafficPolicy= --set-string"

reset_case
HELM_STATE='present'
HELM_OWNED='true'
HELM_STATUS_WARNING='true'
export HELM_STATE HELM_OWNED HELM_STATUS_WARNING
run_action ensure private
assert_success "Helm status warning does not corrupt rerun"
assert_output "Kubernetes configuration file is group-readable"
assert_call "helm upgrade --install contour"

# A first install creates only missing resources with ownership markers and pins.
reset_case
CRDS_STATE='missing'
NAMESPACE_STATE='missing'
COMPONENTS_STATE='missing'
GATEWAYCLASS_STATE='missing'
GATEWAY_STATE='missing'
TARGET_KUBECONFIG_OVERRIDE="${TEST_ROOT}/target-kubeconfig"
touch "${TARGET_KUBECONFIG_OVERRIDE}"
export CRDS_STATE NAMESPACE_STATE COMPONENTS_STATE
export GATEWAYCLASS_STATE GATEWAY_STATE TARGET_KUBECONFIG_OVERRIDE
run_action ensure
assert_success "first managed install"
assert_call "kubectl --kubeconfig ${TARGET_KUBECONFIG_OVERRIDE} create --dry-run=client"
assert_call "gateway-api/releases/download/v1.2.1/experimental-install.yaml"
assert_call "version 0.1.0"
assert_call "gatewayAPI.manageCRDs=false"
assert_call "envoy.service.externalTrafficPolicy= --set-string"
assert_call "backendtlspolicies.gateway.networking.k8s.io"
assert_call "radius-project.io/routes-gateway-lifecycle: v1"
assert_no_call "wait --for=condition=Accepted gatewayclass/contour"
assert_call "wait --for=condition=Programmed gateway/radius"

# Repeated ensures reuse complete unowned resources without adopting them.
reset_case
run_action ensure
assert_success "idempotent reuse"
assert_output "Reusing complete pre-existing Contour resources without adopting"
assert_no_call "helm upgrade"
assert_no_call "kubectl create"

# Conflicts and readiness failures are surfaced.
reset_case
GATEWAYCLASS_CONTROLLER='other.example/controller'
export GATEWAYCLASS_CONTROLLER
run_action ensure
assert_failure "GatewayClass conflict"
assert_output "GatewayClass contour conflicts"

reset_case
WAIT_FAIL='gateway/radius'
export WAIT_FAIL
run_action ensure
assert_failure "Gateway readiness"
assert_output "Gateway radius did not become Programmed"

reset_case
GATEWAY_LISTENERS_VALID='false'
HELM_STATE='present'
HELM_OWNED='true'
export GATEWAY_LISTENERS_VALID HELM_STATE HELM_OWNED
run_action ensure
assert_failure "Gateway listener conflict"
assert_output "Gateway radius conflicts"
assert_no_call "kubectl create"
assert_no_call "helm upgrade"

reset_case
ENDPOINTS_READY='false'
export ENDPOINTS_READY
run_action ensure
assert_failure "Service readiness"
assert_output "has no ready endpoints"

# Cleanup retains while routes remain and fails closed on discovery errors.
reset_case
PACK_SOURCE='br:example.test/custom/routes:v1'
export PACK_SOURCE
run_action cleanup
assert_success "custom recipe cleanup"
assert_output "Custom routes recipe detected"
assert_no_call "kubectl"

reset_case
APP_LIST_JSON="$(jq -nc '[{name:"remaining"}]')"
ROUTES_APPS='remaining'
export APP_LIST_JSON ROUTES_APPS
run_action cleanup
assert_success "retain remaining routes"
assert_output "still uses Radius.Compute/routes"
assert_no_call "kubectl delete"

reset_case
RAD_DISCOVERY_FAIL='app'
export RAD_DISCOVERY_FAIL
run_action cleanup
assert_failure "cleanup discovery failure"
assert_output "preserving Gateway infrastructure"
assert_no_call "kubectl delete"

reset_case
APP_LIST_JSON="$(jq -nc '{}')"
export APP_LIST_JSON
run_action cleanup
assert_failure "malformed application discovery"
assert_output "invalid JSON"
assert_no_call "kubectl delete"

reset_case
APP_LIST_JSON='null'
export APP_LIST_JSON
run_action cleanup
assert_success "empty application discovery"

reset_case
APP_LIST_JSON="$(jq -nc '[{name:"empty"}]')"
RESOURCE_JSON_OVERRIDE='null'
export APP_LIST_JSON RESOURCE_JSON_OVERRIDE
run_action cleanup
assert_success "empty resource discovery"

reset_case
APP_LIST_JSON="$(jq -nc '[{name:"broken"}]')"
RESOURCE_JSON_OVERRIDE="$(jq -nc '{unexpected:true}')"
export APP_LIST_JSON RESOURCE_JSON_OVERRIDE
run_action cleanup
assert_failure "malformed resource discovery"
assert_output "invalid JSON"
assert_no_call "kubectl delete"

# Helm release state and ownership are established before cleanup mutation.
reset_case
HELM_STATE='failure'
export HELM_STATE
run_action cleanup
assert_failure "Helm release discovery failure"
assert_output "failed to inspect Contour Helm release"
assert_no_call "kubectl delete"

reset_case
HELM_STATE='malformed'
export HELM_STATE
run_action cleanup
assert_failure "malformed Helm release status"
assert_output "Contour Helm release status returned invalid JSON"
assert_no_call "kubectl delete"

reset_case
HELM_STATE='present'
HELM_VALUES_STATE='failure'
export HELM_STATE HELM_VALUES_STATE
run_action cleanup
assert_failure "Helm release ownership discovery failure"
assert_output "failed to inspect Contour Helm release ownership"
assert_no_call "kubectl delete"

reset_case
HELM_STATE='present'
export HELM_STATE
run_action cleanup
assert_success "preserve unowned Helm release"
assert_output "Contour Helm release is not Radius-owned"
assert_no_call "kubectl delete"

# Confirmed missing CRDs no-op, while discovery uncertainty fails closed.
reset_case
CRDS_STATE='missing'
export CRDS_STATE
run_action cleanup
assert_success "missing CRDs cleanup"
assert_output "CRDs are absent or incomplete"
assert_no_call "kubectl delete"

reset_case
CRDS_STATE='failure'
export CRDS_STATE
run_action cleanup
assert_failure "CRD discovery failure"
assert_output "failed to inspect Gateway API CRD"
assert_no_call "kubectl delete"

reset_case
CRDS_STATE='malformed'
export CRDS_STATE
run_action cleanup
assert_failure "malformed CRD discovery"
assert_output "CRD gatewayclasses.gateway.networking.k8s.io discovery returned unexpected output"
assert_no_call "kubectl delete"

# Shared Gateway API objects retain controller and CRDs after owned Gateway delete.
reset_case
SHARED_OBJECTS='true'
SHARED_OBJECT_RESOURCE='gatewayclasses.gateway.networking.k8s.io'
export SHARED_OBJECTS SHARED_OBJECT_RESOURCE
run_action cleanup
assert_success "retain shared infrastructure"
assert_output "retaining all Gateway infrastructure"
assert_no_call "kubectl delete gateway radius"
assert_no_call "helm uninstall"
assert_no_call "customresourcedefinition gatewayclasses"

reset_case
SHARED_INGRESS='true'
export SHARED_INGRESS
run_action cleanup
assert_success "retain shared Ingress controller"
assert_output "Ingress or Contour objects use the controller"
assert_no_call "kubectl delete"
assert_no_call "helm uninstall"

reset_case
CONTOUR_CRDS_STATE='present'
CONTOUR_SHARED_OBJECTS='true'
export CONTOUR_CRDS_STATE CONTOUR_SHARED_OBJECTS
run_action cleanup
assert_success "retain shared Contour controller"
assert_output "Ingress or Contour objects use the controller"
assert_no_call "kubectl delete"
assert_no_call "helm uninstall"

reset_case
CONTOUR_CRDS_STATE='failure'
export CONTOUR_CRDS_STATE
run_action cleanup
assert_failure "Contour discovery failure"
assert_output "preserving Gateway infrastructure"
assert_no_call "kubectl delete"

reset_case
CLUSTER_JSON_OVERRIDE="$(jq -nc '{items:"invalid"}')"
export CLUSTER_JSON_OVERRIDE
run_action cleanup
assert_failure "malformed cluster discovery"
assert_output "cluster-wide gatewayclasses.gateway.networking.k8s.io discovery returned invalid JSON"
assert_no_call "kubectl delete"

# Fully unused owned infrastructure is removed; pre-existing CRDs are preserved.
reset_case
HELM_STATE='present'
HELM_OWNED='true'
CRDS_OWNED='true'
GATEWAY_RESOURCES_OWNED='true'
export HELM_STATE HELM_OWNED CRDS_OWNED GATEWAY_RESOURCES_OWNED
run_action cleanup
assert_success "remove owned infrastructure"
assert_call "kubectl delete gateway radius"
assert_call "kubectl delete gatewayclass contour"
assert_call "helm uninstall contour"
assert_call "kubectl delete customresourcedefinition gatewayclasses.gateway.networking.k8s.io"
assert_call "kubectl delete customresourcedefinition backendtlspolicies.gateway.networking.k8s.io"
inventory_line="$(
    grep -n 'kubectl get gatewayclasses.gateway.networking.k8s.io -A -o json' \
        "${CALLS}" | head -1 | cut -d: -f1
)"
delete_line="$(
    grep -n 'kubectl delete gateway radius' "${CALLS}" |
        head -1 | cut -d: -f1
)"
((inventory_line < delete_line)) ||
    fail "foreign-object inventory must run before Gateway deletion"

reset_case
HELM_STATE='present'
HELM_OWNED='true'
CRDS_OWNED='false'
export HELM_STATE HELM_OWNED CRDS_OWNED
run_action cleanup
assert_success "preserve pre-existing CRDs"
assert_no_call "kubectl delete customresourcedefinition"

# BYO cleanup never mutates the cluster.
reset_case
run_action cleanup "" true byo byo-system
assert_success "BYO cleanup"
assert_output "BYO Gateway is never deleted"
assert_no_call "kubectl"
assert_no_call "helm "

# Both provider workflows invoke ensure after recipe setup and before app deploy.
for workflow in "${AZURE_WORKFLOW}" "${AWS_WORKFLOW}"; do
    apply_line="$(grep -n 'name: Apply custom recipe packs' "${workflow}" |
        cut -d: -f1)"
    ensure_line="$(grep -n 'name: Ensure routes Gateway infrastructure' \
        "${workflow}" | cut -d: -f1)"
    deploy_line="$(grep -n 'name: Run rad commands' "${workflow}" |
        cut -d: -f1)"
    ((apply_line < ensure_line && ensure_line < deploy_line)) ||
        fail "${workflow}: Gateway ensure ordering is incorrect"
    # shellcheck disable=SC2016
    grep -qF 'gateway-explicit: "${{ vars.RADIUS_ROUTES_GATEWAY_NAME != '\'''\'' }}"' \
        "${workflow}" ||
        fail "${workflow}: explicit Gateway boolean is not preserved"
done

grep -qF "radius_contrib_kube_recipe_source Radius.Compute/routes routes" \
    "${AWS_WORKFLOW}" ||
    fail "AWS route recipe must use the Radius default OCI recipe"
grep -qF "gatewayName: '\${{ vars.RADIUS_ROUTES_GATEWAY_NAME || 'radius' }}'" \
    "${AWS_WORKFLOW}" ||
    fail "AWS route recipe is missing the managed Gateway default"

for workflow in \
    "${REPO_ROOT}/.github/extension/delete-azure.yml" \
    "${REPO_ROOT}/.github/extension/delete-aws.yml"; do
    delete_line="$(grep -n 'name: Delete Radius resource' "${workflow}" |
        cut -d: -f1)"
    cleanup_line="$(
        grep -n 'name: Clean up unused routes Gateway infrastructure' \
            "${workflow}" | cut -d: -f1
    )"
    teardown_line="$(grep -n 'name: Teardown' "${workflow}" | cut -d: -f1)"
    ((delete_line < cleanup_line && cleanup_line < teardown_line)) ||
        fail "${workflow}: Gateway cleanup ordering is incorrect"
    grep -qF "if: \${{ inputs.resource_type == 'application' }}" \
        "${workflow}" ||
        fail "${workflow}: cleanup must run only for application deletes"
done

for workflow in \
    "${AZURE_WORKFLOW}" \
    "${AWS_WORKFLOW}" \
    "${REPO_ROOT}/.github/extension/delete-azure.yml" \
    "${REPO_ROOT}/.github/extension/delete-aws.yml"; do
    # shellcheck disable=SC2016
    grep -qF 'group: radius-environment-${{ github.repository }}-${{ inputs.environment }}' \
        "${workflow}" ||
        fail "${workflow}: missing shared environment concurrency group"
    grep -qF 'cancel-in-progress: false' "${workflow}" ||
        fail "${workflow}: environment concurrency must not cancel in-progress runs"
done

echo "routes Gateway lifecycle tests passed"
