#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly SCRIPT_PATH="${SCRIPT_DIR}/promote-container-image.sh"

WORKDIR=""
cleanup() {
    if [[ -n "${WORKDIR}" && -d "${WORKDIR}" ]]; then
        rm -rf "${WORKDIR}"
    fi
}
trap cleanup EXIT

WORKDIR="$(mktemp -d)"
readonly CALLS_FILE="${WORKDIR}/calls"
export CALLS_FILE

cat > "${WORKDIR}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

echo "$*" >> "${CALLS_FILE}"
if [[ "$*" == *"imagetools inspect"* ]]; then
    case "${DOCKER_TEST_MODE}" in
        success)
            exit 0
            ;;
        missing)
            echo "ERROR: source: manifest unknown" >&2
            exit 1
            ;;
        auth)
            echo "ERROR: failed to authorize: 403 Forbidden" >&2
            exit 1
            ;;
        network)
            echo "ERROR: failed to do request: connection timed out" >&2
            exit 1
            ;;
    esac
fi
EOF
chmod +x "${WORKDIR}/docker"

run_promote() {
    local mode="$1"
    PATH="${WORKDIR}:${PATH}" \
        DOCKER_TEST_MODE="${mode}" \
        bash "${SCRIPT_PATH}" registry/image:source registry/image:target
}

run_promote success
grep -q 'imagetools create --tag registry/image:target registry/image:source' \
    "${CALLS_FILE}"

: > "${CALLS_FILE}"
run_promote missing
if grep -q 'imagetools create' "${CALLS_FILE}"; then
    echo "Missing source image should not be promoted" >&2
    exit 1
fi

: > "${CALLS_FILE}"
for failure_mode in auth network; do
    failure_output=""
    if failure_output="$(run_promote "${failure_mode}" 2>&1)"; then
        echo "${failure_mode} failures must fail promotion" >&2
        exit 1
    fi
    if [[ -z "${failure_output}" ]]; then
        echo "${failure_mode} failures must preserve the Docker error" >&2
        exit 1
    fi
done