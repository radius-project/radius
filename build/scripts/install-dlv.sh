#!/usr/bin/env bash

set -euo pipefail

# Installs Delve (the 'dlv' debugger) for the current platform using 'go install'.
# Unlike the other install-*.sh scripts, Delve publishes no prebuilt release
# binaries (its GitHub releases contain only source archives), so it is built
# from source with the Go toolchain. Module integrity is guaranteed by the Go
# checksum database (the 'go install <module>@<version>' verification against
# sum.golang.org), so there are no per-platform SHA-256 checksums to pin. Under
# GitHub Actions the Go bin dir is added to the job PATH so later steps can run
# dlv.
#
# The pinned version is normally provided by build/tools.yaml through the
# environment. When it is not supplied it defaults to the latest release.
#
# Usage: install-dlv.sh [install_dir]
#
# Environment (all optional):
#   DLV_VERSION       Module version, e.g. v1.27.0. Empty (or 'latest') selects
#                     the latest release.
#   DLV_INSTALL_DIR   Install directory for the 'dlv' binary. Default: Go's own
#                     install target ($(go env GOBIN), else $(go env GOPATH)/bin).

readonly MODULE="github.com/go-delve/delve/cmd/dlv"

log() { echo "[install-dlv] $*" >&2; }
fail() {
    echo "[install-dlv] ERROR: $*" >&2
    exit 1
}

main() {
    local install_dir version

    command -v go >/dev/null 2>&1 || fail "go is required but was not found"

    # Normalize the requested version: strip whitespace, treat empty as 'latest'.
    version="${DLV_VERSION:-}"
    version="${version//[[:space:]]/}"
    [ -n "$version" ] || version="latest"

    # Determine where 'go install' places the binary so we can force that location,
    # add it to PATH, and run an already-installed check. GOBIN wins when set,
    # otherwise Go uses GOPATH/bin.
    install_dir="${1:-${DLV_INSTALL_DIR:-}}"
    if [ -z "$install_dir" ]; then
        install_dir="$(go env GOBIN)"
        [ -n "$install_dir" ] || install_dir="$(go env GOPATH)/bin"
    fi

    # Skip if the exact pinned version is already installed. 'dlv version' prints a
    # line like 'Version: 1.27.0'; compare against the tag without a leading 'v'.
    if [ "$version" != "latest" ] && command -v dlv >/dev/null 2>&1 \
        && dlv version 2>/dev/null | grep -q "Version: ${version#v}$"; then
        log "dlv ${version} already installed: $(command -v dlv)"
        return 0
    fi

    log "installing dlv ${version} via go install (${MODULE}@${version})..."
    GOBIN="$install_dir" go install "${MODULE}@${version}" \
        || fail "go install ${MODULE}@${version} failed"

    "${install_dir}/dlv" version >/dev/null 2>&1 \
        || fail "installed dlv failed to run (${install_dir}/dlv)"
    log "installed dlv ${version} to ${install_dir}/dlv"

    # Make dlv available to later GitHub Actions steps.
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$install_dir" >> "$GITHUB_PATH"
    fi
}

main "$@"
