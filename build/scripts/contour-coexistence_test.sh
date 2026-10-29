#!/bin/bash

# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0.
# You may obtain a copy of the License at
# http://www.apache.org/licenses/LICENSE-2.0

# Mock infrastructure commands, but run the runner's real jq/HTTP assertions.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/radius-coexistence-test.XXXXXXXX")"
trap 'rm -rf "${work}"' EXIT
mkdir -p "${work}/bin" "${work}/tmp" "${work}/home"
export MOCK_LOG="${work}/commands" MOCK_SCENARIO=""
export HOME="${work}/home" KUBECONFIG="${work}/caller-kubeconfig"
printf 'caller context\n' >"${KUBECONFIG}"
mkdir -p "${HOME}/.rad"
printf 'caller workspace\n' >"${HOME}/.rad/config.yaml"
export CALLER_HOME="${HOME}" CALLER_KUBECONFIG="${KUBECONFIG}"
export TMPDIR="${work}/tmp"

fail() {
    cat "${work}/output" >&2
    echo "FAIL: $*" >&2
    exit 1
}

# An allowlisted PATH prevents a mock regression from reaching real infrastructure.
for tool in bash dirname mktemp mkdir rm tr cat grep diff jq sleep; do
    ln -s "$(command -v "${tool}")" "${work}/bin/${tool}"
done
rm "${work}/bin/sleep"
cat >"${work}/bin/mock" <<'EOF'
#!/bin/bash
set -euo pipefail
tool="${0##*/}"
printf '%s %s\n' "${tool}" "$*" >>"${MOCK_LOG}"
state="${KUBECONFIG%/*}"
cluster="rad-coexist-$(printf '%s' "${state##*.}" | tr '[:upper:]' '[:lower:]')"
case "${tool}" in
    sleep) ;;
    docker)
        case "$1" in
            info) ;;
            inspect)
                echo '[{"NetworkSettings":{"Ports":{
                    "80/tcp":[{"HostIp":"127.0.0.1","HostPort":"19080"}],
                    "8080/tcp":[{"HostIp":"127.0.0.1","HostPort":"19081"}],
                    "6443/tcp":[{"HostIp":"127.0.0.1","HostPort":"19643"}]
                }}}]'
                ;;
            *) exit 99 ;;
        esac
        ;;
    k3d)
        case "$1 $2" in
            "cluster list")
                if [[ "${MOCK_SCENARIO}" == collision ]]; then
                    printf '[{"name":"%s"}]\n' "${cluster}"
                else
                    echo '[]'
                fi
                ;;
            "cluster create")
                [[ "${MOCK_SCENARIO}" != create-failure ]] || exit 42
                ;;
            "kubeconfig get") echo 'private kubeconfig' ;;
            "cluster delete")
                [[ "$3" == "${cluster}" && $# == 3 ]] || exit 99
                [[ "${MOCK_SCENARIO}" != cleanup-failure ]] || exit 42
                ;;
            *) exit 99 ;;
        esac
        ;;
    helm)
        [[ "$1 $2" == \
            "pull oci://ghcr.io/radius-project/helm-chart/radius" ]] || exit 99
        [[ "${HELM_CONFIG_HOME}" == "${state}/helm/config" ]] || exit 99
        [[ "${MOCK_SCENARIO}" != download-failure ]] || exit 42
        ;;
    rad)
        [[ "$1 $2" == "install kubernetes" ]] || exit 99
        [[ "${HOME}" == "${state}/home" &&
            "${HOME}" != "${CALLER_HOME}" ]] || exit 99
        [[ "$*" == *"--chart ${state}/radius-0.62.0.tgz"* &&
            "$*" == *"--contour-chart ${state}/contour-0.1.0.tgz"* ]] || exit 99
        printf 'installed\n' >"${MOCK_LOG}.installed"
        case "${MOCK_SCENARIO}" in
            install-failure) exit 42 ;;
            signal) kill -TERM "${PPID}"; exit 143 ;;
        esac
        ;;
    curl)
        output=""
        request="$*"
        while (($#)); do
            case "$1" in
                --output | --dump-header) : >"$2"; shift 2 ;;
                *) shift ;;
            esac
        done
        if [[ "${request}" == *https://* ]]; then
            printf '200 https://chart.test\n'
        elif [[ "${request}" == *'Host: radius.test'* &&
            "${MOCK_SCENARIO}" =~ ^(no-exposure|diagnostics-failure)$ ]]; then
            exit 7
        elif [[ "${request}" == *'Host: radius.test'* &&
            "${MOCK_SCENARIO}" == wrong-body ]]; then
            echo '404 from the wrong ingress controller'
        elif [[ "${request}" == *'Host: traefik.test'* &&
            "${MOCK_SCENARIO}" == traefik-after &&
            -f "${MOCK_LOG}.installed" ]]; then
            exit 22
        else
            echo 'Hostname: coexistence-backend'
        fi
        ;;
    kubectl)
        [[ "$1" == --kubeconfig && "$2" == "${KUBECONFIG}" &&
            "$2" != "${CALLER_KUBECONFIG}" ]] || exit 99
        shift 3
        case "$*" in
            "config set-cluster k3d-${cluster} --server=https://127.0.0.1:19643")
                ;;
            "get crd -o json")
                owner=traefik-crd
                if [[ "${MOCK_SCENARIO}" == crd-takeover &&
                    -f "${MOCK_LOG}.installed" ]]; then
                    owner=contour
                fi
                jq -n --arg owner "${owner}" '{items:[{
                    spec:{group:"gateway.networking.k8s.io"},
                    metadata:{name:"httproutes.gateway.networking.k8s.io",
                        uid:"original-crd",
                        annotations:{"meta.helm.sh/release-name":$owner,
                            "meta.helm.sh/release-namespace":"kube-system"},
                        labels:{"app.kubernetes.io/managed-by":"Helm"}}
                }]}'
                ;;
            "-n radius-system get service contour-envoy -o json")
                port=8080
                address='[{"ip":"10.0.0.1"}]'
                [[ "${MOCK_SCENARIO}" != wrong-ports ]] || port=80
                [[ "${MOCK_SCENARIO}" != pending-lb ]] || address='[]'
                [[ "${MOCK_SCENARIO}" != empty-lb ]] ||
                    address='[{"ip":""}]'
                jq -n --argjson port "${port}" --argjson address "${address}" \
                    '{spec:{type:"LoadBalancer",ports:[
                        {name:"http",port:$port},{name:"https",port:8443}]},
                      status:{loadBalancer:{ingress:$address}}}'
                ;;
            "-n radius-system get daemonset contour-envoy -o json")
                host=false
                [[ "${MOCK_SCENARIO}" != host-network ]] || host=true
                jq -n --argjson host "${host}" \
                    '{spec:{template:{spec:{hostNetwork:$host,containers:[]}}}}'
                ;;
            "get gatewayclass contour -o json")
                echo '{"spec":{"controllerName":"projectcontour.io/gateway-controller"}}'
                ;;
            "-n ingress-probe get httproute radius-route -o json")
                controller=projectcontour.io/gateway-controller
                accepted=True
                resolved=True
                generation=1
                case "${MOCK_SCENARIO}" in
                    wrong-controller) controller=traefik.io/gateway-controller ;;
                    rejected-route) accepted=False ;;
                    unresolved-route) resolved=False ;;
                    stale-route) generation=0 ;;
                esac
                jq -n --arg controller "${controller}" \
                    --arg accepted "${accepted}" --arg resolved "${resolved}" \
                    --argjson generation "${generation}" \
                    '{metadata:{generation:1},status:{parents:[{
                        controllerName:$controller,
                        parentRef:{name:"radius",namespace:"radius-system",
                            sectionName:"http"},
                        conditions:[
                            {type:"Accepted",status:$accepted,
                                observedGeneration:$generation},
                            {type:"ResolvedRefs",status:$resolved,
                                observedGeneration:$generation}]
                    }]}}'
                ;;
            *"rollout status"* | *" wait "* | "apply -f "* | \
                *"get service contour-envoy -o wide") ;;
            "get pods,services"* | "get events"* | "get gateway,httproute"* | \
                "logs -n radius-system deployment/contour-contour"*)
                [[ "${MOCK_SCENARIO}" != diagnostics-failure ]] || exit 42
                ;;
            *) echo "Unexpected kubectl: $*" >&2; exit 99 ;;
        esac
        ;;
    *) echo "Unexpected tool: ${tool}" >&2; exit 99 ;;
esac
EOF
chmod +x "${work}/bin/mock"
for tool in docker k3d kubectl helm rad curl sleep; do
    ln -s mock "${work}/bin/${tool}"
done
export PATH="${work}/bin"

run_case() {
    local scenario="$1" expected="$2" status=0
    export MOCK_SCENARIO="${scenario}"
    : >"${MOCK_LOG}"
    rm -f "${MOCK_LOG}.installed"
    bash "${SCRIPT_DIR}/contour-coexistence.sh" "${work}/bin/rad" \
        >"${work}/output" 2>&1 || status=$?
    if [[ "${expected}" == pass ]]; then
        ((status == 0)) || fail "${scenario}: unexpected status ${status}"
        grep -Fq 'PASS: external Traefik and Contour HTTP' "${work}/output" ||
            fail "${scenario}: missing HTTP proof"
    else
        ((status != 0)) || fail "${scenario}: false success"
    fi
    if [[ "${scenario}" == no-exposure ]]; then
        [[ "$(grep -Fc 'Host: radius.test' "${MOCK_LOG}")" == 30 ]] ||
            fail "${scenario}: incorrect HTTP retry bound"
    fi
    [[ "$(cat "${CALLER_KUBECONFIG}")" == "caller context" &&
        "$(cat "${CALLER_HOME}/.rad/config.yaml")" == "caller workspace" ]] ||
        fail "${scenario}: changed caller configuration"
    shopt -s nullglob
    local remaining=("${TMPDIR}"/*)
    ((${#remaining[@]} == 0)) || fail "${scenario}: leaked temporary files"
    case "${scenario}" in
        collision | download-failure)
            if grep -Fq 'k3d cluster delete' "${MOCK_LOG}"; then
                fail "${scenario}: deleted a cluster we did not create"
            fi
            ;;
        *)
            grep -Fq 'k3d cluster delete rad-coexist-' "${MOCK_LOG}" ||
                fail "${scenario}: did not clean up cluster"
            ;;
    esac
    echo "PASS: ${scenario}"
}

run_case success pass
for expected in \
    '--kubeconfig-update-default=false --kubeconfig-switch-context=false' \
    '--image rancher/k3s:v1.35.5-k3s1' \
    '--port 127.0.0.1:0:80@loadbalancer' \
    '--port 127.0.0.1:0:8080@loadbalancer' \
    '--contour-set contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}' \
    '--contour-set envoy.hostNetwork=false,envoy.service.ports.http=8080,envoy.service.ports.https=8443' \
    '--chart ' '--contour-chart ' \
    'Host: radius.test http://127.0.0.1:19081/' \
    'Host: traefik.test http://127.0.0.1:19080/'; do
    grep -Fq -- "${expected}" "${MOCK_LOG}" ||
        fail "missing command propagation: ${expected}"
done
if grep -Eq -- '--disable=traefik|helm (install|upgrade)|--all' "${MOCK_LOG}"; then
    fail "unsafe or substitute installation command"
fi

for scenario in collision download-failure create-failure install-failure \
    signal cleanup-failure no-exposure wrong-body traefik-after wrong-ports \
    pending-lb empty-lb host-network wrong-controller rejected-route unresolved-route \
    stale-route crd-takeover diagnostics-failure; do
    run_case "${scenario}" fail
done
echo "All coexistence runner tests passed."
