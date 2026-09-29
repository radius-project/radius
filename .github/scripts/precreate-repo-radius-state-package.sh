#!/bin/bash

# Creates or verifies the private GHCR package used by the Repo Radius state
# rehydration workflow.

set -euo pipefail

readonly SCRIPT_NAME="$(basename "$0")"
readonly BOOTSTRAP_TAG="bootstrap"
readonly BOOTSTRAP_ARTIFACT_TYPE="application/vnd.radius.statearchive.bootstrap.v1"

PACKAGE=""
SOURCE_REPOSITORY=""
WORK_DIR=""

usage() {
    cat <<EOF
Usage: ${SCRIPT_NAME} --package <ghcr.io/owner/package> \\
  --source-repository <https://github.com/owner/repository>

Creates a harmless bootstrap version when the package is absent, then verifies
that the package is private or internal and linked to the source repository.

Prerequisites:
  - gh authenticated as a user allowed to create the package
  - the active gh token includes write:packages
  - oras and jq are installed
EOF
}

fail() {
    echo "[precreate-state-package] ERROR: $*" >&2
    exit 1
}

log() {
    echo "[precreate-state-package] $*"
}

cleanup() {
    if [[ -n "${WORK_DIR}" && -d "${WORK_DIR}" ]]; then
        rm -rf "${WORK_DIR}"
    fi
}

trap cleanup EXIT

urlencode() {
    jq -nr --arg value "$1" '$value | @uri'
}

parse_args() {
    while (($# > 0)); do
        case "$1" in
            --package)
                [[ $# -ge 2 ]] || fail "--package requires a value"
                PACKAGE="$2"
                shift 2
                ;;
            --source-repository)
                [[ $# -ge 2 ]] \
                    || fail "--source-repository requires a value"
                SOURCE_REPOSITORY="$2"
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *)
                fail "unknown argument: $1"
                ;;
        esac
    done

    [[ -n "${PACKAGE}" ]] || fail "--package is required"
    [[ -n "${SOURCE_REPOSITORY}" ]] \
        || fail "--source-repository is required"
}

require_tools() {
    local tool
    for tool in gh oras jq; do
        command -v "${tool}" >/dev/null 2>&1 \
            || fail "${tool} is required"
    done
}

parse_package() {
    if [[ ! "${PACKAGE}" =~ ^ghcr\.io/([^/]+)/(.+)$ ]]; then
        fail "package must match ghcr.io/<owner>/<package>"
    fi
    PACKAGE_OWNER="${BASH_REMATCH[1]}"
    PACKAGE_NAME="${BASH_REMATCH[2]}"

    if [[ ! "${PACKAGE_OWNER}" =~ ^[A-Za-z0-9][A-Za-z0-9-]*$ ]]; then
        fail "invalid GHCR owner: ${PACKAGE_OWNER}"
    fi
    if [[ ! "${PACKAGE_NAME}" =~ ^[A-Za-z0-9][A-Za-z0-9._/-]*$ ]]; then
        fail "invalid GHCR package name: ${PACKAGE_NAME}"
    fi

    local source="${SOURCE_REPOSITORY%.git}"
    source="${source%/}"
    if [[ ! "${source}" =~ ^https://github\.com/([^/]+)/([^/]+)$ ]]; then
        fail "source repository must match https://github.com/<owner>/<repo>"
    fi
    SOURCE_REPOSITORY="${source}"
    SOURCE_REPOSITORY_FULL_NAME="${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
}

configure_package_api() {
    local owner_type owner_path package_path
    owner_type="$(gh api "/users/${PACKAGE_OWNER}" --jq '.type')"
    case "${owner_type}" in
        Organization) owner_path="orgs" ;;
        User) owner_path="users" ;;
        *) fail "unsupported owner type ${owner_type} for ${PACKAGE_OWNER}" ;;
    esac

    package_path="$(urlencode "${PACKAGE_NAME}")"
    PACKAGE_API="/${owner_path}/$(urlencode "${PACKAGE_OWNER}")/packages/container/${package_path}"
}

package_exists() {
    local output
    if output="$(gh api "${PACKAGE_API}" 2>&1)"; then
        printf '%s' "${output}" >"${WORK_DIR}/package.json"
        return 0
    fi
    if grep -Eqi '(^|[^0-9])404([^0-9]|$)|package not found|not found' \
        <<<"${output}"; then
        return 1
    fi
    echo "${output}" >&2
    fail "could not query package metadata"
}

login_to_ghcr() {
    local username token
    username="$(gh api /user --jq '.login')"
    token="$(gh auth token)" \
        || fail "could not read the active gh token"
    printf '%s' "${token}" \
        | oras login ghcr.io --username "${username}" --password-stdin
}

push_bootstrap() {
    mkdir -p "${WORK_DIR}/artifact"
    printf '%s\n' \
        "Harmless bootstrap for the Repo Radius state package." \
        >"${WORK_DIR}/artifact/bootstrap.txt"

    (
        cd "${WORK_DIR}/artifact"
        oras push "${PACKAGE}:${BOOTSTRAP_TAG}" \
            --artifact-type "${BOOTSTRAP_ARTIFACT_TYPE}" \
            --annotation \
            "org.opencontainers.image.source=${SOURCE_REPOSITORY}" \
            "bootstrap.txt:text/plain"
    )
}

wait_for_package() {
    local _
    for _ in {1..20}; do
        if package_exists; then
            return 0
        fi
        sleep 2
    done
    fail "package metadata did not become available after bootstrap"
}

verify_package() {
    local visibility linked_repository
    visibility="$(jq -r '.visibility // empty' \
        "${WORK_DIR}/package.json")"
    case "${visibility}" in
        private | internal) ;;
        public)
            fail "package ${PACKAGE} is public; change its name or visibility before storing Radius state"
            ;;
        *) fail "unsupported package visibility: ${visibility:-missing}" ;;
    esac

    linked_repository="$(jq -r '.repository.full_name // empty' \
        "${WORK_DIR}/package.json")"
    if [[ ! "${linked_repository,,}" == \
        "${SOURCE_REPOSITORY_FULL_NAME,,}" ]]; then
        fail "package is linked to ${linked_repository:-no repository}; expected ${SOURCE_REPOSITORY_FULL_NAME}"
    fi
}

ensure_bootstrap() {
    if oras manifest fetch --descriptor \
        "${PACKAGE}:${BOOTSTRAP_TAG}" >"${WORK_DIR}/bootstrap.json" 2>/dev/null; then
        return 0
    fi

    log "bootstrap tag is missing; adding harmless bootstrap artifact"
    login_to_ghcr
    push_bootstrap
    oras manifest fetch --descriptor \
        "${PACKAGE}:${BOOTSTRAP_TAG}" >"${WORK_DIR}/bootstrap.json"
}

print_result() {
    local digest package_url
    digest="$(jq -r '.digest' "${WORK_DIR}/bootstrap.json")"
    package_url="$(jq -r '.html_url' "${WORK_DIR}/package.json")"

    cat <<EOF
Package ready:
  Package:    ${PACKAGE}
  Visibility: $(jq -r '.visibility' "${WORK_DIR}/package.json")
  Repository: $(jq -r '.repository.full_name' "${WORK_DIR}/package.json")
  Bootstrap:  ${digest}
  URL:        ${package_url}

Run the workflow:
  gh workflow run repo-radius-state-e2e.yaml \\
    --repo ${SOURCE_REPOSITORY_FULL_NAME} \\
    -f state_package=${PACKAGE_NAME}
EOF
}

main() {
    parse_args "$@"
    require_tools
    parse_package
    WORK_DIR="$(mktemp -d)"
    configure_package_api

    if package_exists; then
        log "package exists; verifying configuration"
        verify_package
        login_to_ghcr
        ensure_bootstrap
    else
        log "package does not exist; creating harmless bootstrap"
        login_to_ghcr
        push_bootstrap
        wait_for_package
        verify_package
        oras manifest fetch --descriptor \
            "${PACKAGE}:${BOOTSTRAP_TAG}" >"${WORK_DIR}/bootstrap.json"
    fi

    print_result
}

main "$@"
