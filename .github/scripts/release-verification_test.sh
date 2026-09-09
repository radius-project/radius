#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REAL_HELM="$(command -v helm)"
TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "${TEST_ROOT}"' EXIT
export TEST_ROOT
mkdir -p "${TEST_ROOT}/bin"
cat > "${TEST_ROOT}/rad" << 'EOF'
#!/bin/bash
set -euo pipefail
echo "$*" >>"${TEST_ROOT}/rad-calls"
if [[ "$1" == "version" ]]; then
    printf '{"version":"v0.61.0","release":"0.61.0"}\n'
elif [[ "${FAIL_INSTALL:-false}" == "true" ]]; then
    exit 1
fi
EOF
cat > "${TEST_ROOT}/bin/kind" << 'EOF'
#!/bin/bash
echo "$*" >>"${TEST_ROOT}/kind-calls"
EOF
cat > "${TEST_ROOT}/bin/helm" << 'EOF'
#!/bin/bash
echo "$*" >>"${TEST_ROOT}/helm-calls"
[[ "$*" == *"--set preupgrade.checks.version=false"* ]] || exit 1
EOF
cat > "${TEST_ROOT}/bin/curl" << 'EOF'
#!/bin/bash
echo "Staged verification must not download a public binary" >&2
exit 1
EOF
cat > "${TEST_ROOT}/bin/kubectl" << 'EOF'
#!/bin/bash
set -euo pipefail
case "$*" in
    "wait "*) exit 0 ;;
    *"--no-headers"*) printf '%s\n' applications-rp bicep-de controller dashboard dynamic-rp ucp ;;
    "get pod "*)
        name="$5"
        case "${name}" in
            bicep-de) name=deployment-engine ;;
            ucp) name=ucpd ;;
        esac
        jq -r --arg name "${name}" '.observed.images[] | select(.name == $name) | .reference + "@" + .digest' "${TEST_ROOT}/manifest.json"
        ;;
    "get job pre-upgrade "*)
        jq '{spec:{template:{spec:{containers:[{image:(.observed.images[] | select(.name == "pre-upgrade") | .reference + "@" + .digest)}]}}}}' "${TEST_ROOT}/manifest.json"
        ;;
    "get pods "*)
        jq '{items:[{spec:{initContainers:[{image:(.observed.images[] | select(.name == "bicep") | .reference + "@" + .digest)}]}}]}' "${TEST_ROOT}/manifest.json"
        ;;
    *) exit 1 ;;
esac
EOF
chmod +x "${TEST_ROOT}/rad" "${TEST_ROOT}/bin/"*
touch "${TEST_ROOT}/radius.tgz"
export PATH="${TEST_ROOT}/bin:${PATH}"
export RELEASE_VERIFY_CLI="${TEST_ROOT}/rad"
export RELEASE_VERIFY_CHART="${TEST_ROOT}/radius.tgz"
export RELEASE_VERIFY_MANIFEST="${TEST_ROOT}/manifest.json"
jq -n --arg checksum "$(sha256sum "${TEST_ROOT}/rad" | cut -d ' ' -f 1)" \
    --arg chartDigest "sha256:$(sha256sum "${TEST_ROOT}/radius.tgz" | cut -d ' ' -f 1)" '
    {tag:"v0.61.0",checks:{plan:"verified",assets:"verified",metadata:"verified",
      images:"verified",helm:"verified",external:"verified",installation:"pending"},
    observed:{helm:{manifest:{layers:[{mediaType:"application/vnd.cncf.helm.chart.content.v1.tar+gzip",digest:$chartDigest}]}},cli:{assets:[{name:"rad_linux_amd64",sha256:$checksum}]},images:
      (["applications-rp","controller","ucpd","dynamic-rp","dashboard",
        "deployment-engine","bicep","pre-upgrade"] | map({name:.,
        reference:("ghcr.io/radius-project/" + . + ":0.61.0"),
        digest:("sha256:" + ("c" * 64))}))}}
' > "${RELEASE_VERIFY_MANIFEST}"
chart_args=()
for component in rp:applications-rp controller:controller ucp:ucpd \
    dynamicrp:dynamic-rp de:deployment-engine dashboard:dashboard \
    bicep:bicep preupgrade:pre-upgrade; do
    image="$(jq -er --arg name "${component#*:}" '
        .observed.images[] | select(.name == $name) |
        .reference + "@" + .digest
    ' "${RELEASE_VERIFY_MANIFEST}")"
    chart_args+=(--set "${component%%:*}.image=${image}")
done
"${REAL_HELM}" template radius "${ROOT}/deploy/Chart" \
    --namespace radius-system --set preupgrade.enabled=true \
    "${chart_args[@]}" > "${TEST_ROOT}/rendered.yaml"
yq -o=json '.' "${TEST_ROOT}/rendered.yaml" | jq -s -e \
    --slurpfile manifest "${RELEASE_VERIFY_MANIFEST}" '
    ([.[] | .. | objects | .image? // empty |
      select(type == "string" and contains("@sha256:cccc"))] | unique | sort)
      == ([$manifest[0].observed.images[] |
        .reference + "@" + .digest] | sort)
' > /dev/null
bash "${ROOT}/.github/scripts/release-verification.sh" 0.61.0 > /dev/null
jq -e '.checks.installation == "verified"' "${RELEASE_VERIFY_MANIFEST}" > /dev/null
[[ -f "${TEST_ROOT}/rad" ]] || {
    echo "Input CLI was deleted"
    exit 1
}
grep -Fq -- "--chart ${TEST_ROOT}/radius.tgz" "${TEST_ROOT}/rad-calls"
[[ "$(grep -o '@sha256:' "${TEST_ROOT}/rad-calls" | wc -l)" == "8" ]]
grep -Fq -- 'delete cluster --name radius-verification-' "${TEST_ROOT}/kind-calls"
if FAIL_INSTALL=true bash "${ROOT}/.github/scripts/release-verification.sh" 0.61.0 > /dev/null 2>&1; then
    echo "A failed installation was accepted" >&2
    exit 1
fi
jq -e '.checks.installation != "verified"' "${RELEASE_VERIFY_MANIFEST}" > /dev/null
printf 'changed\n' > "${TEST_ROOT}/radius.tgz"
if bash "${ROOT}/.github/scripts/release-verification.sh" 0.61.0 > /dev/null 2>&1; then
    echo "A changed chart was accepted" >&2
    exit 1
fi
truncate -s 0 "${TEST_ROOT}/radius.tgz"
jq '.observed.cli.assets[0].sha256 = "wrong"' "${RELEASE_VERIFY_MANIFEST}" > "${TEST_ROOT}/bad.json"
mv "${TEST_ROOT}/bad.json" "${RELEASE_VERIFY_MANIFEST}"
if bash "${ROOT}/.github/scripts/release-verification.sh" 0.61.0 > /dev/null 2>&1; then
    echo "A changed staged binary was accepted" >&2
    exit 1
fi
echo "Staged installation tests passed (5 tests)"
