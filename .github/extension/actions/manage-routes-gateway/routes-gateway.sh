#!/bin/bash

# Manages the target-cluster Gateway API lifecycle for Radius.Compute/routes.

set -euo pipefail

readonly CONTOUR_CHART_REPO="https://projectcontour.github.io/helm-charts"
readonly CONTOUR_CHART_VERSION="0.1.0"
readonly CONTOUR_RELEASE="contour"
readonly GATEWAY_API_VERSION="v1.2.1"
readonly GATEWAY_API_URL="https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/experimental-install.yaml"
readonly MANAGED_BY="radius-repo"
readonly MANAGED_LABEL="app.kubernetes.io/managed-by"
readonly OWNERSHIP_ANNOTATION="radius-project.io/routes-gateway-lifecycle"
readonly OWNERSHIP_VALUE="v1"
readonly DEFAULT_GATEWAY_NAME="radius"
readonly DEFAULT_GATEWAY_NAMESPACE="radius-system"
readonly DEFAULT_GATEWAY_CLASS="contour"
readonly CONTOUR_CONTROLLER="projectcontour.io/gateway-controller"
readonly DEFAULT_ROUTES_SOURCE_PREFIX="ghcr.io/radius-project/kube-recipes/routes:"
readonly WAIT_TIMEOUT="${RADIUS_ROUTES_WAIT_TIMEOUT:-5m}"
readonly RETRY_DELAY="${RADIUS_ROUTES_RETRY_DELAY:-5}"
readonly RETRY_ATTEMPTS="${RADIUS_ROUTES_RETRY_ATTEMPTS:-12}"

readonly -a REQUIRED_CRDS=(
    "gatewayclasses.gateway.networking.k8s.io"
    "gateways.gateway.networking.k8s.io"
    "httproutes.gateway.networking.k8s.io"
    "referencegrants.gateway.networking.k8s.io"
    "grpcroutes.gateway.networking.k8s.io"
    "tcproutes.gateway.networking.k8s.io"
    "tlsroutes.gateway.networking.k8s.io"
    "udproutes.gateway.networking.k8s.io"
)
readonly -a GATEWAY_API_OBJECTS=(
    "gatewayclasses.gateway.networking.k8s.io"
    "gateways.gateway.networking.k8s.io"
    "httproutes.gateway.networking.k8s.io"
    "grpcroutes.gateway.networking.k8s.io"
    "tcproutes.gateway.networking.k8s.io"
    "tlsroutes.gateway.networking.k8s.io"
    "udproutes.gateway.networking.k8s.io"
    "referencegrants.gateway.networking.k8s.io"
)

MODE="${MODE:-}"
TARGET_KUBECONFIG="${TARGET_KUBECONFIG:-}"
ENVIRONMENT_NAME="${ENVIRONMENT_NAME:-}"
APPLICATION_NAME="${APPLICATION_NAME:-}"
APP_FILE="${APP_FILE:-}"
GATEWAY_NAME="${GATEWAY_NAME-}"
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE-}"
GATEWAY_EXPLICIT="${GATEWAY_EXPLICIT:-false}"
EXPOSURE="${EXPOSURE:-}"

declare -a KUBE_ARGS=()
declare -a HELM_KUBE_ARGS=()

TEMP_DIR=""

cleanup_temp() {
    if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
        rm -rf "${TEMP_DIR}"
    fi
}
trap cleanup_temp EXIT

fail() {
    echo "::error::$*" >&2
    exit 1
}

kube() {
    kubectl "${KUBE_ARGS[@]}" "$@"
}

helm_target() {
    helm "${HELM_KUBE_ARGS[@]}" "$@"
}

require_detection_tools() {
    local command
    for command in jq rad; do
        command -v "${command}" >/dev/null 2>&1 ||
            fail "${command} is required to manage the routes Gateway"
    done
}

require_gateway_tools() {
    local command
    for command in helm kubectl; do
        command -v "${command}" >/dev/null 2>&1 ||
            fail "${command} is required to manage the routes Gateway"
    done
    if [[ "${MODE}" == "ensure" ]]; then
        command -v curl >/dev/null 2>&1 ||
            fail "curl is required to manage the routes Gateway"
    fi
}

validate_basic_inputs() {
    case "${MODE}" in
        ensure | cleanup) ;;
        *) fail "mode must be 'ensure' or 'cleanup', got '${MODE}'" ;;
    esac
    [[ -n "${ENVIRONMENT_NAME}" ]] ||
        fail "environment-name is required"
}

validate_gateway_inputs() {
    case "${GATEWAY_EXPLICIT}" in
        true | false) ;;
        *)
            fail "gateway-explicit must be 'true' or 'false', got '${GATEWAY_EXPLICIT}'"
            ;;
    esac

    case "${EXPOSURE}" in
        "" | private | public) ;;
        *)
            fail "RADIUS_ROUTES_EXPOSURE must be 'private' or 'public', got '${EXPOSURE}'"
            ;;
    esac

    if [[ "${GATEWAY_EXPLICIT}" == "true" ]]; then
        [[ -n "${GATEWAY_NAME}" ]] ||
            fail "RADIUS_ROUTES_GATEWAY_NAME cannot be empty when explicitly configured"
        [[ -n "${GATEWAY_NAMESPACE}" ]] ||
            fail "RADIUS_ROUTES_GATEWAY_NAMESPACE is required with RADIUS_ROUTES_GATEWAY_NAME"
        if [[ -n "${EXPOSURE}" ]]; then
            fail "RADIUS_ROUTES_EXPOSURE cannot be set with an explicit BYO Gateway"
        fi
    else
        if [[ -n "${GATEWAY_NAMESPACE}" ]]; then
            fail "RADIUS_ROUTES_GATEWAY_NAMESPACE cannot be set without RADIUS_ROUTES_GATEWAY_NAME"
        fi
        GATEWAY_NAME="${DEFAULT_GATEWAY_NAME}"
        GATEWAY_NAMESPACE="${DEFAULT_GATEWAY_NAMESPACE}"
    fi

    if [[ -n "${TARGET_KUBECONFIG}" ]]; then
        if [[ ! -f "${TARGET_KUBECONFIG}" ||
            ! -r "${TARGET_KUBECONFIG}" ]]; then
            fail "target kubeconfig is not a readable regular file at ${TARGET_KUBECONFIG}"
        fi
        KUBE_ARGS=(--kubeconfig "${TARGET_KUBECONFIG}")
        HELM_KUBE_ARGS=(--kubeconfig "${TARGET_KUBECONFIG}")
    fi
}

compile_app() {
    local bicep_bin="${BICEP_BIN:-${BICEP:-${HOME}/.rad/bin/bicep}}"
    local output="$1"

    [[ -n "${APP_FILE}" ]] || fail "app-file is required in ensure mode"
    [[ -f "${APP_FILE}" ]] || fail "application file not found at ${APP_FILE}"
    if [[ -d "${bicep_bin}" ]]; then
        bicep_bin="${bicep_bin}/bicep"
    fi
    [[ -x "${bicep_bin}" ]] ||
        fail "Bicep compiler not found at ${bicep_bin}"
    "${bicep_bin}" build "${APP_FILE}" --outfile "${output}" ||
        fail "failed to compile ${APP_FILE} before routes Gateway detection"
}

app_declares_routes() {
    local compiled="$1"
    jq -e '
      def resource_values:
        if type == "object" or type == "array" then
          .[]
        else
          error("compiled template resources must be an object or array")
        end;
      def is_nested_deployment:
        ((.type // "") | ascii_downcase) ==
          "microsoft.resources/deployments";
      def validate_resources:
        if type != "object" and type != "array" then
          error("compiled template resources must be an object or array")
        else
          [
            resource_values |
            if type != "object" then
              error("compiled template resource entries must be objects")
            elif (
              is_nested_deployment and
              (.properties | type) == "object" and
              (.properties | has("template"))
            ) then
              if (.properties.template | type) != "object" then
                error("nested deployment template must be an object")
              else
                .properties.template.resources | validate_resources
              end
            else
              true
            end
          ] |
          true
        end;
      def declares_routes:
        any(
          resource_values;
          if (
            (((.type // "") | ascii_downcase) |
              . == "radius.compute/routes" or
              startswith("radius.compute/routes@")) and
            ((.existing // false) != true)
          ) then
            true
          elif (
            is_nested_deployment and
            (.properties | type) == "object" and
            (.properties | has("template"))
          ) then
            if (.properties.template | type) != "object" then
              error("nested deployment template must be an object")
            else
              .properties.template.resources | declares_routes
            end
          else
            false
          end
        );
      .resources as $resources |
      if ($resources | validate_resources) then
        $resources | declares_routes
      else
        false
      end
    ' "${compiled}" >/dev/null
}

environment_recipe_pack_ids_json() {
    local environment_json
    environment_json="$(
        rad env show "${ENVIRONMENT_NAME}" --preview -o json
    )" || fail "failed to inspect Radius environment ${ENVIRONMENT_NAME}"
    jq -sc '
      if length == 0 then
        error("environment lookup returned no JSON")
      elif (.[0].properties.recipePacks | type) != "array" then
        error("environment recipePacks must be an array")
      elif any(.[0].properties.recipePacks[];
               type != "string" or length == 0) then
        error("environment recipePacks contains an invalid resource ID")
      else
        .[0].properties.recipePacks
      end
    ' <<<"${environment_json}" ||
        fail "environment '${ENVIRONMENT_NAME}' returned invalid recipe-pack data"
}

effective_routes_source() {
    local pack_ids
    local pack_id
    local pack_json
    local route_source

    pack_ids="$(environment_recipe_pack_ids_json)"
    while IFS= read -r pack_id; do
        [[ -n "${pack_id}" ]] || continue
        pack_json="$(rad recipe-pack show "${pack_id}" -o json)" ||
            fail "failed to inspect recipe pack ${pack_id}"
        route_source="$(
            jq -sce '
              if length != 1 then
                error("recipe pack lookup must return one JSON document")
              elif (.[0].properties.recipes | type) != "object" then
                error("recipe pack recipes must be an object")
              else
                [
                  .[0].properties.recipes | to_entries[] |
                  select(.key | ascii_downcase == "radius.compute/routes")
                ] |
                if length == 0 then null
                elif length != 1 or
                     (.[0].value.source | type) != "string" or
                     (.[0].value.source | length) == 0 then
                  error("routes recipe must have one non-empty string source")
                else
                  .[0].value.source
                end
              end
            ' <<<"${pack_json}"
        )" || fail "recipe pack ${pack_id} returned invalid recipe data"
        if [[ "${route_source}" != "null" ]]; then
            jq -r . <<<"${route_source}"
            return 0
        fi
    done < <(jq -r '.[]' <<<"${pack_ids}")

    return 1
}

uses_default_routes_recipe() {
    local source
    source="$(effective_routes_source)" ||
        fail "environment '${ENVIRONMENT_NAME}' has no effective Radius.Compute/routes recipe"
    if [[ "${source}" == "${DEFAULT_ROUTES_SOURCE_PREFIX}"* &&
        "${source#"${DEFAULT_ROUTES_SOURCE_PREFIX}"}" != "" ]]; then
        echo "Effective routes recipe is the Radius default: ${source}"
        return 0
    fi
    echo "Custom routes recipe detected (${source}); Gateway lifecycle is a no-op."
    return 1
}

gateway_crd_state() {
    local crd="$1"
    local result

    if result="$(
        kube get "customresourcedefinition/${crd}" -o name 2>&1
    )"; then
        case "${result}" in
            "customresourcedefinition.apiextensions.k8s.io/${crd}" | \
                "customresourcedefinition/${crd}")
                printf 'present\n'
                ;;
            *)
                fail "Gateway API CRD ${crd} discovery returned unexpected output"
                ;;
        esac
        return 0
    fi
    if grep -Eqi 'not found|notfound' <<<"${result}"; then
        printf 'missing\n'
        return 0
    fi
    printf '%s\n' "${result}" >&2
    fail "failed to inspect Gateway API CRD ${crd}"
}

gateway_crds_state() {
    local crd
    local state
    local incomplete=false

    for crd in "${REQUIRED_CRDS[@]}"; do
        if ! state="$(gateway_crd_state "${crd}")"; then
            return 1
        fi
        if [[ "${state}" == "missing" ]]; then
            incomplete=true
        fi
    done
    if [[ "${incomplete}" == "true" ]]; then
        printf 'incomplete\n'
    else
        printf 'complete\n'
    fi
}

wait_for_crds() {
    local crd
    for crd in "${REQUIRED_CRDS[@]}"; do
        kube wait --for=condition=Established \
            "customresourcedefinition/${crd}" \
            --timeout="${WAIT_TIMEOUT}" ||
            fail "Gateway API CRD ${crd} did not become Established"
    done
}

install_missing_crds() {
    local crd
    local object
    local state
    local manifest="${TEMP_DIR}/gateway-api.yaml"

    curl -fsSL "${GATEWAY_API_URL}" -o "${manifest}" ||
        fail "failed to download Gateway API ${GATEWAY_API_VERSION} CRDs"

    for crd in "${REQUIRED_CRDS[@]}"; do
        state="$(gateway_crd_state "${crd}")" ||
            fail "failed to determine Gateway API CRD ${crd} state"
        if [[ "${state}" == "present" ]]; then
            continue
        fi

        object="$(
            kube create --dry-run=client -f "${manifest}" -o json |
                jq -sc --arg name "${crd}" \
                    --arg label "${MANAGED_LABEL}" \
                    --arg managed_by "${MANAGED_BY}" \
                    --arg annotation "${OWNERSHIP_ANNOTATION}" \
                    --arg value "${OWNERSHIP_VALUE}" '
                      [
                        .[] |
                        if .kind == "List" then .items[] else . end |
                        select(.kind == "CustomResourceDefinition" and
                               .metadata.name == $name) |
                        .metadata.labels[$label] = $managed_by |
                        .metadata.annotations[$annotation] = $value
                      ][0] // error("CRD not found in pinned Gateway API manifest")
                    '
        )" || fail "failed to render Gateway API CRD ${crd}"

        if ! printf '%s\n' "${object}" | kube create -f -; then
            state="$(gateway_crd_state "${crd}")" ||
                fail "failed to determine Gateway API CRD ${crd} state"
            [[ "${state}" == "present" ]] ||
                fail "failed to create Gateway API CRD ${crd}"
        fi
    done
    wait_for_crds
}

resource_owned() {
    local resource="$1"
    local namespace="${2:-}"
    local name="$3"
    local value
    declare -a namespace_args=()
    if [[ -n "${namespace}" ]]; then
        namespace_args=(-n "${namespace}")
    fi
    value="$(
        kube get "${resource}" "${name}" "${namespace_args[@]}" \
            -o "jsonpath={.metadata.annotations.${OWNERSHIP_ANNOTATION//./\\.}}" \
            2>/dev/null || true
    )"
    [[ "${value}" == "${OWNERSHIP_VALUE}" ]]
}

create_namespace_if_missing() {
    if kube get namespace "${DEFAULT_GATEWAY_NAMESPACE}" >/dev/null 2>&1; then
        return 0
    fi
    if ! cat <<EOF | kube create -f -
apiVersion: v1
kind: Namespace
metadata:
  name: ${DEFAULT_GATEWAY_NAMESPACE}
  labels:
    ${MANAGED_LABEL}: ${MANAGED_BY}
  annotations:
    ${OWNERSHIP_ANNOTATION}: ${OWNERSHIP_VALUE}
EOF
    then
        kube get namespace "${DEFAULT_GATEWAY_NAMESPACE}" >/dev/null 2>&1 ||
            fail "failed to create namespace ${DEFAULT_GATEWAY_NAMESPACE}"
    fi
}

contour_release_state() {
    local stderr_file="${TEMP_DIR}/contour-status.stderr"
    local status
    local values

    if status="$(
        helm_target status "${CONTOUR_RELEASE}" \
            -n "${DEFAULT_GATEWAY_NAMESPACE}" -o json 2>"${stderr_file}"
    )"; then
        if [[ -s "${stderr_file}" ]]; then
            cat "${stderr_file}" >&2
        fi
        jq -e --arg name "${CONTOUR_RELEASE}" \
            --arg namespace "${DEFAULT_GATEWAY_NAMESPACE}" '
              type == "object" and
              .name == $name and
              .namespace == $namespace
            ' <<<"${status}" >/dev/null ||
            fail "Contour Helm release status returned invalid JSON"
    elif grep -Eqi \
        'release: not found|release: "[^"]+" not found' \
        "${stderr_file}"; then
        printf 'absent\n'
        return 0
    else
        cat "${stderr_file}" >&2
        if [[ -n "${status}" ]]; then
            printf '%s\n' "${status}" >&2
        fi
        fail "failed to inspect Contour Helm release"
    fi

    values="$(
        helm_target get values "${CONTOUR_RELEASE}" \
            -n "${DEFAULT_GATEWAY_NAMESPACE}" -o json
    )" || fail "failed to inspect Contour Helm release ownership"
    jq -e 'type == "object"' <<<"${values}" >/dev/null ||
        fail "Contour Helm release values returned invalid JSON"
    if jq -e --arg key "${OWNERSHIP_ANNOTATION}" \
        --arg value "${OWNERSHIP_VALUE}" \
        '.commonAnnotations[$key] == $value' <<<"${values}" >/dev/null; then
        printf 'owned\n'
    else
        printf 'unowned\n'
    fi
}

contour_components_state() {
    local deployments
    local services
    deployments="$(
        kube get deployment -n "${DEFAULT_GATEWAY_NAMESPACE}" \
            -l app.kubernetes.io/component=contour -o json |
            jq '.items | length'
    )" || fail "failed to discover existing Contour deployments"
    services="$(
        kube get service -n "${DEFAULT_GATEWAY_NAMESPACE}" \
            -l app.kubernetes.io/component=envoy -o json |
            jq '.items | length'
    )" || fail "failed to discover existing Envoy Services"
    printf '%s:%s\n' "${deployments}" "${services}"
}

desired_service_type() {
    if [[ "${EXPOSURE}" == "public" ]]; then
        printf 'LoadBalancer\n'
    else
        printf 'ClusterIP\n'
    fi
}

upgrade_managed_contour() {
    local service_type
    local attempt
    service_type="$(desired_service_type)"
    for ((attempt = 1; attempt <= RETRY_ATTEMPTS; attempt++)); do
        if helm_target upgrade --install "${CONTOUR_RELEASE}" contour \
            --repo "${CONTOUR_CHART_REPO}" \
            --version "${CONTOUR_CHART_VERSION}" \
            --namespace "${DEFAULT_GATEWAY_NAMESPACE}" \
            --set gatewayAPI.manageCRDs=false \
            --set-string \
            "configInline.gateway.gatewayRef.name=${DEFAULT_GATEWAY_NAME}" \
            --set-string \
            "configInline.gateway.gatewayRef.namespace=${DEFAULT_GATEWAY_NAMESPACE}" \
            --set-string "envoy.service.type=${service_type}" \
            --set-string \
            "commonLabels.app\\.kubernetes\\.io/managed-by=${MANAGED_BY}" \
            --set-string \
            "commonAnnotations.radius-project\\.io/routes-gateway-lifecycle=${OWNERSHIP_VALUE}" \
            --wait --timeout "${WAIT_TIMEOUT}"; then
            return 0
        fi
        if ((attempt < RETRY_ATTEMPTS)); then
            echo "Contour reconciliation is busy; retrying (${attempt}/${RETRY_ATTEMPTS})."
            sleep "${RETRY_DELAY}"
        fi
    done
    return 1
}

ensure_contour() {
    local release_state
    local state
    release_state="$(contour_release_state)" ||
        fail "failed to determine Contour Helm release state"
    case "${release_state}" in
        owned)
            upgrade_managed_contour
            return 0
            ;;
        absent | unowned) ;;
        *) fail "unexpected Contour Helm release state: ${release_state}" ;;
    esac

    state="$(contour_components_state)"
    if [[ "${release_state}" == "unowned" ]]; then
        [[ "${state}" == "1:1" ]] ||
            fail "pre-existing Contour release is incomplete and is not Radius-owned"
        return 0
    fi

    case "${state}" in
        0:0)
            if ! upgrade_managed_contour; then
                release_state="$(contour_release_state)" ||
                    fail "failed to determine Contour Helm release state"
                if [[ "${release_state}" == "owned" ]]; then
                    upgrade_managed_contour
                else
                    fail "failed to install pinned Contour ${CONTOUR_CHART_VERSION}"
                fi
            fi
            ;;
        1:1)
            echo "Reusing complete pre-existing Contour resources without adopting them."
            ;;
        *)
            fail "pre-existing Contour resources are incomplete; refusing to adopt them"
            ;;
    esac
}

gateway_class_valid() {
    local class_name="$1"
    local expected_controller="${2:-}"
    local controller
    controller="$(
        kube get gatewayclass "${class_name}" \
            -o jsonpath='{.spec.controllerName}' 2>/dev/null
    )" || return 1
    if [[ -n "${expected_controller}" &&
        "${controller}" != "${expected_controller}" ]]; then
        return 1
    fi
    kube wait --for=condition=Accepted "gatewayclass/${class_name}" \
        --timeout="${WAIT_TIMEOUT}" >/dev/null
}

managed_gateway_spec_valid() {
    local gateway_json="$1"
    jq -e --arg expected_class "${DEFAULT_GATEWAY_CLASS}" '
      (.spec.listeners // []) as $listeners |
      .spec.gatewayClassName == $expected_class and
      ([
        $listeners[] |
        select(
          .name == "http" and
          .protocol == "HTTP" and
          .port == 80 and
          .allowedRoutes.namespaces.from == "All"
        )
      ] | length) == 1 and
      ([
        $listeners[] |
        select(
          .name == "tls" and
          .protocol == "TLS" and
          .port == 443 and
          .tls.mode == "Passthrough" and
          .allowedRoutes.namespaces.from == "All"
        )
      ] | length) == 1
    ' <<<"${gateway_json}" >/dev/null
}

gateway_valid() {
    local name="$1"
    local namespace="$2"
    local expected_class="${3:-}"
    local gateway_json
    local class_name
    gateway_json="$(
        kube get gateway "${name}" -n "${namespace}" -o json 2>/dev/null
    )" || return 1
    class_name="$(jq -er '.spec.gatewayClassName' <<<"${gateway_json}")" ||
        return 1
    if [[ -n "${expected_class}" &&
        "${class_name}" != "${expected_class}" ]]; then
        return 1
    fi
    if [[ -n "${expected_class}" ]] &&
        ! managed_gateway_spec_valid "${gateway_json}"; then
        return 1
    fi
    gateway_class_valid "${class_name}" &&
        kube wait --for=condition=Programmed \
            "gateway/${name}" -n "${namespace}" \
            --timeout="${WAIT_TIMEOUT}" >/dev/null
}

validate_managed_gateway_conflicts() {
    local controller
    local gateway_json

    if ! kube get \
        "customresourcedefinition/gateways.gateway.networking.k8s.io" \
        >/dev/null 2>&1; then
        return 0
    fi

    if controller="$(
        kube get gatewayclass "${DEFAULT_GATEWAY_CLASS}" \
            -o jsonpath='{.spec.controllerName}' 2>/dev/null
    )"; then
        [[ "${controller}" == "${CONTOUR_CONTROLLER}" ]] ||
            fail "GatewayClass contour conflicts with the Radius Contour controller"
    fi

    if gateway_json="$(
        kube get gateway "${DEFAULT_GATEWAY_NAME}" \
            -n "${DEFAULT_GATEWAY_NAMESPACE}" -o json 2>/dev/null
    )"; then
        managed_gateway_spec_valid "${gateway_json}" ||
            fail "Gateway radius conflicts with the required Radius listeners"
    fi
}

validate_byo() {
    local crd_state
    crd_state="$(gateway_crds_state)" ||
        fail "BYO Gateway validation failed: could not inspect Gateway API CRDs"
    [[ "${crd_state}" == "complete" ]] ||
        fail "BYO Gateway validation failed: Gateway API CRDs are incomplete"
    wait_for_crds
    gateway_valid "${GATEWAY_NAME}" "${GATEWAY_NAMESPACE}" ||
        fail "BYO Gateway validation failed: Gateway, GatewayClass, or controller is not ready"
    echo "BYO Gateway ${GATEWAY_NAMESPACE}/${GATEWAY_NAME} is ready; no resources changed."
}

create_gateway_class_if_missing() {
    if kube get gatewayclass "${DEFAULT_GATEWAY_CLASS}" >/dev/null 2>&1; then
        gateway_class_valid \
            "${DEFAULT_GATEWAY_CLASS}" "${CONTOUR_CONTROLLER}" ||
            fail "GatewayClass contour conflicts with the Radius Contour controller"
        return 0
    fi
    if ! cat <<EOF | kube create -f -
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: ${DEFAULT_GATEWAY_CLASS}
  labels:
    ${MANAGED_LABEL}: ${MANAGED_BY}
  annotations:
    ${OWNERSHIP_ANNOTATION}: ${OWNERSHIP_VALUE}
spec:
  controllerName: ${CONTOUR_CONTROLLER}
EOF
    then
        kube get gatewayclass "${DEFAULT_GATEWAY_CLASS}" >/dev/null 2>&1 ||
            fail "failed to create GatewayClass ${DEFAULT_GATEWAY_CLASS}"
    fi
    gateway_class_valid \
        "${DEFAULT_GATEWAY_CLASS}" "${CONTOUR_CONTROLLER}" ||
        fail "GatewayClass contour was not accepted by Contour"
}

create_gateway_if_missing() {
    if kube get gateway "${DEFAULT_GATEWAY_NAME}" \
        -n "${DEFAULT_GATEWAY_NAMESPACE}" >/dev/null 2>&1; then
        gateway_valid "${DEFAULT_GATEWAY_NAME}" \
            "${DEFAULT_GATEWAY_NAMESPACE}" "${DEFAULT_GATEWAY_CLASS}" ||
            fail "Gateway radius conflicts with the Radius Contour Gateway"
        return 0
    fi
    if ! cat <<EOF | kube create -f -
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: ${DEFAULT_GATEWAY_NAME}
  namespace: ${DEFAULT_GATEWAY_NAMESPACE}
  labels:
    ${MANAGED_LABEL}: ${MANAGED_BY}
  annotations:
    ${OWNERSHIP_ANNOTATION}: ${OWNERSHIP_VALUE}
spec:
  gatewayClassName: ${DEFAULT_GATEWAY_CLASS}
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
    - name: tls
      protocol: TLS
      port: 443
      tls:
        mode: Passthrough
      allowedRoutes:
        namespaces:
          from: All
EOF
    then
        kube get gateway "${DEFAULT_GATEWAY_NAME}" \
            -n "${DEFAULT_GATEWAY_NAMESPACE}" >/dev/null 2>&1 ||
            fail "failed to create Gateway radius"
    fi
    gateway_valid "${DEFAULT_GATEWAY_NAME}" \
        "${DEFAULT_GATEWAY_NAMESPACE}" "${DEFAULT_GATEWAY_CLASS}" ||
        fail "Gateway radius did not become Programmed"
}

envoy_service_json() {
    kube get service -n "${DEFAULT_GATEWAY_NAMESPACE}" \
        -l app.kubernetes.io/component=envoy -o json |
        jq -e '
          if (.items | length) == 1 then .items[0]
          else error("expected exactly one Envoy Service")
          end
        '
}

validate_envoy_service() {
    local expected_type
    local service
    local service_name
    local actual_type
    expected_type="$(desired_service_type)"
    service="$(envoy_service_json)" ||
        fail "failed to find the Contour Envoy Service"
    service_name="$(jq -r '.metadata.name' <<<"${service}")"
    actual_type="$(jq -r '.spec.type' <<<"${service}")"
    [[ "${actual_type}" == "${expected_type}" ]] ||
        fail "Envoy Service ${service_name} is ${actual_type}; expected ${expected_type}"

    kube get endpoints "${service_name}" \
        -n "${DEFAULT_GATEWAY_NAMESPACE}" -o json |
        jq -e 'any(.subsets[]?.addresses[]?; .ip != null)' >/dev/null ||
        fail "Envoy Service ${service_name} has no ready endpoints"

    if [[ "${expected_type}" == "LoadBalancer" ]]; then
        kube wait \
            --for=jsonpath='{.status.loadBalancer.ingress[0]}' \
            "service/${service_name}" \
            -n "${DEFAULT_GATEWAY_NAMESPACE}" \
            --timeout="${WAIT_TIMEOUT}" ||
            fail "public Envoy Service ${service_name} has no load balancer address"
    fi
}

normalize_items_json() {
    local json="$1"
    local context="$2"
    jq -sce '
      if length != 1 then
        error("expected one JSON document")
      elif (.[0] | type) == "array" then .[0]
      elif (.[0].items | type) == "array" then .[0].items
      elif (.[0].value | type) == "array" then .[0].value
      else error("expected an array, items array, or value array")
      end
    ' <<<"${json}" ||
        fail "${context} returned invalid JSON"
}

radius_app_names_json() {
    local apps_json
    apps_json="$(rad app list --preview -o json)" ||
        fail "failed to list Radius applications"
    apps_json="$(normalize_items_json "${apps_json}" "Radius application discovery")"
    jq -ce '
      if any(.[];
             type != "object" or
             ((.name // .properties.name) | type) != "string" or
             ((.name // .properties.name) | length) == 0) then
        error("application entries require non-empty names")
      else
        map(.name // .properties.name)
      end
    ' <<<"${apps_json}" ||
        fail "Radius application discovery returned invalid application entries"
}

app_resources_json() {
    local app="$1"
    local resources_json
    resources_json="$(
        rad resource list --preview --application "${app}" -o json
    )" || fail "failed to inspect resources for Radius application ${app}"
    resources_json="$(
        normalize_items_json \
            "${resources_json}" "resource discovery for application ${app}"
    )"
    jq -ce '
      if any(.[];
             type != "object" or
             ((.type // .resourceType) | type) != "string" or
             ((.type // .resourceType) | length) == 0) then
        error("resource entries require non-empty types")
      else
        .
      end
    ' <<<"${resources_json}" ||
        fail "resource discovery for application ${app} returned invalid entries"
}

resources_include_routes() {
    local resources_json="$1"
    jq -e '
      any(.[];
        ((.type // .resourceType) | ascii_downcase) as $type |
        $type == "radius.compute/routes" or
        ($type | startswith("radius.compute/routes@")))
    ' <<<"${resources_json}" >/dev/null
}

radius_apps_using_routes() {
    local apps
    local resources_json
    local app
    apps="$(radius_app_names_json)" ||
        fail "failed to discover applications for the public exposure warning"
    while IFS= read -r app; do
        [[ -n "${app}" ]] || continue
        resources_json="$(app_resources_json "${app}")" ||
            fail "failed to inspect ${app} for the public exposure warning"
        if resources_include_routes "${resources_json}"; then
            printf '%s\n' "${app}"
        fi
    done < <(jq -r '.[]' <<<"${apps}")
}

warn_public_exposure() {
    local apps
    apps="$(radius_apps_using_routes)" ||
        fail "failed to identify applications affected by public exposure"
    if [[ -z "${apps}" ]]; then
        apps="(none currently deployed)"
    else
        apps="$(paste -sd ',' - <<<"${apps}" | sed 's/,/, /g')"
    fi
    echo "::warning::PUBLIC ROUTES EXPOSURE: the shared Gateway will expose every Radius application using routes. Affected applications: ${apps}"
}

remaining_routes_exist() {
    local apps
    local resources_json
    local app
    apps="$(radius_app_names_json)" ||
        fail "failed to discover remaining Radius applications; preserving Gateway infrastructure"
    while IFS= read -r app; do
        [[ -n "${app}" ]] || continue
        resources_json="$(app_resources_json "${app}")" ||
            fail "failed to inspect ${app}; preserving Gateway infrastructure"
        if resources_include_routes "${resources_json}"; then
            echo "Application ${app} still uses Radius.Compute/routes."
            return 0
        fi
    done < <(jq -r '.[]' <<<"${apps}")
    return 1
}

non_radius_gateway_objects_exist() {
    local resource
    local objects
    for resource in "${GATEWAY_API_OBJECTS[@]}"; do
        objects="$(kube get "${resource}" -A -o json)" ||
            fail "failed to inspect cluster-wide ${resource}; preserving Gateway infrastructure"
        objects="$(
            normalize_items_json \
                "${objects}" "cluster-wide ${resource} discovery"
        )" || fail "cluster-wide ${resource} discovery returned invalid JSON"
        objects="$(
            jq -ce '
              if any(.[];
                     type != "object" or
                     (.metadata | type) != "object" or
                     ((.metadata.annotations // {}) | type) != "object") then
                error("Gateway API entries require metadata and annotations")
              else
                .
              end
            ' <<<"${objects}"
        )" || fail "cluster-wide ${resource} discovery returned invalid entries"
        if jq -e --arg annotation "${OWNERSHIP_ANNOTATION}" \
            --arg value "${OWNERSHIP_VALUE}" '
              any(.[];
                (.metadata.annotations[$annotation] // "") != $value)
            ' <<<"${objects}" >/dev/null; then
            return 0
        fi
    done
    return 1
}

objects_exist() {
    local resource="$1"
    local context="$2"
    local objects
    objects="$(kube get "${resource}" -A -o json)" ||
        fail "failed to inspect ${context}; preserving Gateway infrastructure"
    objects="$(normalize_items_json "${objects}" "${context}")" ||
        fail "${context} returned invalid JSON"
    jq -e 'length > 0' <<<"${objects}" >/dev/null
}

optional_contour_objects_exist() {
    local resource="$1"
    local crd="$2"
    local context="$3"
    local crd_result

    if crd_result="$(
        kube get customresourcedefinition "${crd}" -o json 2>&1
    )"; then
        objects_exist "${resource}" "${context}"
        return $?
    fi
    if grep -Eqi 'not found|notfound' <<<"${crd_result}"; then
        return 1
    fi
    fail "failed to discover ${crd}; preserving Gateway infrastructure"
}

shared_contour_consumers_exist() {
    if objects_exist \
        "ingresses.networking.k8s.io" "cluster-wide Kubernetes Ingresses"; then
        return 0
    fi
    if optional_contour_objects_exist \
        "httpproxies.projectcontour.io" \
        "httpproxies.projectcontour.io" \
        "cluster-wide Contour HTTPProxies"; then
        return 0
    fi
    if optional_contour_objects_exist \
        "tlscertificatedelegations.projectcontour.io" \
        "tlscertificatedelegations.projectcontour.io" \
        "cluster-wide Contour TLSCertificateDelegations"; then
        return 0
    fi
    return 1
}

delete_owned_resource() {
    local resource="$1"
    local namespace="${2:-}"
    local name="$3"
    declare -a namespace_args=()
    if [[ -n "${namespace}" ]]; then
        namespace_args=(-n "${namespace}")
    fi
    if ! kube get "${resource}" "${name}" "${namespace_args[@]}" \
        >/dev/null 2>&1; then
        return 0
    fi
    if resource_owned "${resource}" "${namespace}" "${name}"; then
        kube delete "${resource}" "${name}" "${namespace_args[@]}" \
            --ignore-not-found --wait=true
    else
        echo "Preserving pre-existing ${resource} ${namespace:+${namespace}/}${name}."
    fi
}

delete_owned_crds() {
    local crd
    for crd in "${REQUIRED_CRDS[@]}"; do
        if resource_owned customresourcedefinition "" "${crd}"; then
            kube delete customresourcedefinition "${crd}" \
                --ignore-not-found --wait=true
        fi
    done
}

ensure_gateway() {
    local compiled="${TEMP_DIR}/app.json"
    local routes_status
    compile_app "${compiled}"
    set +e
    app_declares_routes "${compiled}"
    routes_status=$?
    set -e
    case "${routes_status}" in
        0) ;;
        1)
            echo "Application declares no non-existing Radius.Compute/routes resources; no-op."
            return 0
            ;;
        *)
            fail "compiled application template has invalid resources data"
            ;;
    esac
    if ! uses_default_routes_recipe; then
        return 0
    fi
    validate_gateway_inputs
    require_gateway_tools
    if [[ "${GATEWAY_EXPLICIT}" == "true" ]]; then
        validate_byo
        return 0
    fi

    validate_managed_gateway_conflicts

    if [[ "${EXPOSURE}" == "public" ]]; then
        warn_public_exposure
    fi

    create_namespace_if_missing
    install_missing_crds
    ensure_contour
    create_gateway_class_if_missing
    create_gateway_if_missing
    validate_envoy_service
    echo "Routes Gateway is ready with $(desired_service_type) exposure."
}

cleanup_gateway() {
    local crd_state
    local release_state

    if ! uses_default_routes_recipe; then
        return 0
    fi
    validate_gateway_inputs
    require_gateway_tools
    if [[ "${GATEWAY_EXPLICIT}" == "true" ]]; then
        echo "BYO Gateway is never deleted by Radius; cleanup is a no-op."
        return 0
    fi
    if remaining_routes_exist; then
        echo "Routes remain; retaining shared Gateway infrastructure."
        return 0
    fi
    release_state="$(contour_release_state)" ||
        fail "failed to determine Contour Helm release state"
    if [[ "${release_state}" == "unowned" ]]; then
        echo "Pre-existing Contour Helm release is not Radius-owned; retaining all Gateway infrastructure."
        return 0
    fi

    crd_state="$(gateway_crds_state)" ||
        fail "failed to determine Gateway API CRD state"
    if [[ "${crd_state}" == "incomplete" ]]; then
        echo "Gateway API CRDs are absent or incomplete; nothing safe to remove."
        return 0
    fi

    if shared_contour_consumers_exist; then
        echo "Ingress or Contour objects use the controller; retaining all Gateway infrastructure."
        return 0
    fi

    if non_radius_gateway_objects_exist; then
        echo "Non-Radius Gateway API objects exist; retaining all Gateway infrastructure."
        return 0
    fi

    delete_owned_resource gateway \
        "${DEFAULT_GATEWAY_NAMESPACE}" "${DEFAULT_GATEWAY_NAME}"

    if non_radius_gateway_objects_exist; then
        echo "Gateway API objects appeared during cleanup; retaining shared controller and CRDs."
        return 0
    fi

    delete_owned_resource gatewayclass "" "${DEFAULT_GATEWAY_CLASS}"
    if [[ "${release_state}" == "owned" ]]; then
        helm_target uninstall "${CONTOUR_RELEASE}" \
            -n "${DEFAULT_GATEWAY_NAMESPACE}" --wait \
            --timeout "${WAIT_TIMEOUT}"
    fi

    if non_radius_gateway_objects_exist; then
        echo "Gateway API objects appeared during cleanup; retaining CRDs."
        return 0
    fi
    delete_owned_crds
    echo "Unused Radius-owned routes Gateway infrastructure removed."
}

main() {
    validate_basic_inputs
    require_detection_tools
    TEMP_DIR="$(mktemp -d)"
    if [[ "${MODE}" == "ensure" ]]; then
        ensure_gateway
    else
        cleanup_gateway
    fi
}

main "$@"
