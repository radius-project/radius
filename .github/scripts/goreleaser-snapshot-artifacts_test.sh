#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_ROOT="$(mktemp -d)"
readonly SCRIPT_DIR TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT
export GORELEASER_DIST_DIR="${TEST_ROOT}/snapshot"
export SNAPSHOT_IMAGES_DIR="${TEST_ROOT}/images"
export DOCKER_REGISTRY=registry.example/radius
export DOCKER_TAG_VERSION=pr-123
export COMMAND_LOG="${TEST_ROOT}/commands"
mkdir -p "${GORELEASER_DIST_DIR}" "${TEST_ROOT}/bin"
printf '%s\n' '{"version":"0.0.0-snapshot-0123456"}' >"${GORELEASER_DIST_DIR}/metadata.json"
jq '[.images[] | select(.category == "production") | .name as $id | .requiredPlatforms[] |
    ("registry.example/radius/" + $id + ":0.0.0-snapshot-0123456-" + gsub("/"; "-")) as $name |
    {type: "Docker Image", name: $name, path: $name, extra: {ID: $id, Platforms: [.]}}]
' "${SCRIPT_DIR}/../release-parity/targets.json" >"${GORELEASER_DIST_DIR}/artifacts.json"
cp "${GORELEASER_DIST_DIR}/artifacts.json" "${TEST_ROOT}/complete.json"
cat >"${TEST_ROOT}/bin/docker" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"${COMMAND_LOG}"
[[ "${FAIL_PUSH:-false}" != true || "$1" != push ]]
EOF
cat >"${TEST_ROOT}/bin/oras" <<'EOF'
#!/bin/bash
set -euo pipefail
[[ "$1" == resolve ]]
printf 'oras %s\n' "$*" >>"${COMMAND_LOG}"
printf 'sha256:%064d\n' 1
EOF
chmod +x "${TEST_ROOT}/bin/"*
export PATH="${TEST_ROOT}/bin:${PATH}"

bash "${SCRIPT_DIR}/goreleaser-snapshot-artifacts.sh" save
[[ "$(grep -c '^docker save ' "${COMMAND_LOG}")" == 5 ]]
[[ "$(grep -c '^docker tag ' "${COMMAND_LOG}")" == 5 ]]
grep -Fq 'registry.example/radius/ucpd:pr-123' "${COMMAND_LOG}"
if grep -Eq 'push|buildx|latest|testrp|magpiego' "${COMMAND_LOG}"; then exit 1; fi

: >"${COMMAND_LOG}"
DOCKER_TAG_VERSION=edge bash "${SCRIPT_DIR}/goreleaser-snapshot-artifacts.sh" push-edge
[[ "$(grep -c '^docker push ' "${COMMAND_LOG}")" == 15 ]]
[[ "$(grep -c '^docker buildx imagetools create ' "${COMMAND_LOG}")" == 5 ]]
grep -Eq '^docker buildx imagetools create --tag registry.example/radius/ucpd:edge( registry.example/radius/ucpd@sha256:[a-f0-9]{64}){3}$' "${COMMAND_LOG}"
if grep -Eq ':latest|:0\.[0-9]+ |testrp|magpiego' "${COMMAND_LOG}"; then exit 1; fi

for mutation in '.[1:]' '. + [.[0]]' '.[0].extra.Platforms = ["linux/386"]' '.[0].path = "unexpected/image:tag"'; do
    jq "${mutation}" "${TEST_ROOT}/complete.json" >"${GORELEASER_DIST_DIR}/artifacts.json"
    : >"${COMMAND_LOG}"
    if bash "${SCRIPT_DIR}/goreleaser-snapshot-artifacts.sh" save; then exit 1; fi
    [[ ! -s "${COMMAND_LOG}" ]]
done
cp "${TEST_ROOT}/complete.json" "${GORELEASER_DIST_DIR}/artifacts.json"
: >"${COMMAND_LOG}"
if DOCKER_TAG_VERSION=edge FAIL_PUSH=true bash "${SCRIPT_DIR}/goreleaser-snapshot-artifacts.sh" push-edge; then exit 1; fi
if grep -q 'imagetools create' "${COMMAND_LOG}"; then exit 1; fi
if DOCKER_TAG_VERSION=latest bash "${SCRIPT_DIR}/goreleaser-snapshot-artifacts.sh" push-edge; then exit 1; fi
echo "GoReleaser snapshot artifact tests passed"