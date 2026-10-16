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
    release="${RAD_RELEASE:-0.61.0}"
    printf '{"version":"v%s","release":"%s"}\n' "${release}" "${release}"
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
if [[ "$1" == "pull" ]]; then
    : > "$6/radius-$4.tgz"
    exit
fi
if [[ "$1" == "template" ]]; then
    # Charts render their version unless CHART_IMAGE_TAG models an older tag scheme.
    version="$(basename "$3" .tgz)"
    for name in applications-rp bicep controller dashboard deployment-engine dynamic-rp pre-upgrade ucpd; do
        printf '          image: "ghcr.io/radius-project/%s:%s"\n' "${name}" "${CHART_IMAGE_TAG:-${version#radius-}}"
    done
    exit
fi
[[ "$*" == *"--set preupgrade.checks.version=false"* ]] || exit 1
EOF
cat > "${TEST_ROOT}/bin/curl" << 'EOF'
#!/bin/bash
if [[ "${FAKE_PUBLIC_INSTALL:-false}" == "true" ]]; then
    cp "${TEST_ROOT}/rad" "${@: -1}"
    exit
fi
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
        if [[ "${FAKE_PUBLIC_INSTALL:-false}" == "true" ]]; then
            # Without --chart, rad installs the newest patch chart in the channel.
            tag="${NEWEST_PATCH}"
            chart='--chart [^ ]*/radius-([^ /]+)\.tgz'
            [[ "$(tail -n 1 "${TEST_ROOT}/rad-calls")" =~ ${chart} ]] && tag="${CHART_IMAGE_TAG:-${BASH_REMATCH[1]}}"
            echo "ghcr.io/radius-project/${name}:${tag}"
            exit
        fi
        jq -r --arg name "${name}" '.observed.images[] | select(.name == $name) | .reference + "@" + .digest' "${TEST_ROOT}/manifest.json"
        ;;
    "get job pre-upgrade "*)
        if [[ "${FAKE_PUBLIC_INSTALL:-false}" == "true" ]]; then
            # The upgrade renders its chart's default tag unless it sets one.
            upgrade="$(tail -n 1 "${TEST_ROOT}/helm-calls")"
            chart="$(basename "$(cut -d ' ' -f 3 <<<"${upgrade}")" .tgz)"
            tag="$(grep -o 'global.imageTag=[^ ]*' <<<"${upgrade}" | cut -d = -f 2 || true)"
            jq -n --arg image "ghcr.io/radius-project/pre-upgrade:${tag:-${CHART_IMAGE_TAG:-${chart#radius-}}}" '{spec:{template:{spec:{containers:[{image:$image}]}}}}'
            exit
        fi
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
FAKE_PUBLIC_INSTALL=true NEWEST_PATCH=0.61.1 RELEASE_VERIFY_CLI="" \
    RELEASE_VERIFY_CHART="" RELEASE_VERIFY_MANIFEST="" \
    bash "${ROOT}/.github/scripts/release-verification.sh" 0.61.0 > /dev/null
pull="$(grep '^pull ' "${TEST_ROOT}/helm-calls")"
[[ "${pull}" == "pull oci://ghcr.io/radius-project/helm-chart/radius --version 0.61.0 --destination "* ]]
chart="${pull##* }/radius-0.61.0.tgz"
[[ "$(tail -n 1 "${TEST_ROOT}/rad-calls")" == *"--chart ${chart}"* ]]
[[ "$(tail -n 1 "${TEST_ROOT}/helm-calls")" == "upgrade radius ${chart} "* ]]
# Charts published before full-version image tags render major.minor tags.
: > "${TEST_ROOT}/helm-calls"
FAKE_PUBLIC_INSTALL=true NEWEST_PATCH=0.61.2 RAD_RELEASE=0.61.1 CHART_IMAGE_TAG=0.61 \
    RELEASE_VERIFY_CLI="" RELEASE_VERIFY_CHART="" RELEASE_VERIFY_MANIFEST="" \
    bash "${ROOT}/.github/scripts/release-verification.sh" 0.61.1 > /dev/null
if grep -q 'global.imageTag' "${TEST_ROOT}/helm-calls"; then
    echo "Public verification overrode the chart's image tag" >&2
    exit 1
fi
for case in 0.61.1:0.60 0.60.0-rc3:0.60; do
    if FAKE_PUBLIC_INSTALL=true NEWEST_PATCH=0.61.2 RAD_RELEASE="${case%%:*}" \
        CHART_IMAGE_TAG="${case#*:}" RELEASE_VERIFY_CLI="" RELEASE_VERIFY_CHART="" \
        RELEASE_VERIFY_MANIFEST="" bash "${ROOT}/.github/scripts/release-verification.sh" \
        "${case%%:*}" > /dev/null 2> "${TEST_ROOT}/stderr"; then
        echo "Chart image tag ${case#*:} was accepted for ${case%%:*}" >&2
        exit 1
    fi
    grep -Fq 'do not match release' "${TEST_ROOT}/stderr"
done
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
echo "Release installation tests passed (8 tests)"
