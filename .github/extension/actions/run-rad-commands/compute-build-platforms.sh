#!/bin/bash

# Computes the effective container image build platforms for a Radius deploy,
# given the requested arch mode, a fallback platform list, and the set of node
# architectures detected on the target cluster.
#
# This exists because Radius.Compute/containerImages builds run in an in-cluster
# BuildKit that is compiled for the runner's architecture (amd64 on standard
# GitHub-hosted runners). When an app leaves build.platforms unset, the recipe
# defaults to a multi-arch build (linux/amd64,linux/arm64), so the arm64 half is
# produced under QEMU emulation -- an order of magnitude slower and prone to
# emulation crashes. When the target cluster is single-arch, building only that
# one platform avoids emulation entirely; when it is mixed (or we cannot tell),
# a multi-arch fallback preserves portability.
#
# Contract (kept intentionally small and explicit):
#   MODE
#     - ''                          Feature disabled: emit nothing so the recipe
#                                   default platforms apply (existing behavior).
#     - '{{TARGET_CLUSTER_ARCH_MODE}}'
#                                   An unsubstituted template placeholder is
#                                   treated the same as empty (disabled).
#     - 'detect'                    Probe the target cluster node architectures.
#     - an explicit platform list   Any value containing '/', e.g. 'linux/amd64'
#                                   or 'linux/amd64,linux/arm64', is honored
#                                   verbatim with no detection.
#   FALLBACK
#     - comma-separated platform list used when detection is inconclusive
#       (mixed-arch or undetermined). Empty or an unsubstituted placeholder
#       defaults to 'linux/amd64,linux/arm64'.
#   ARCHES
#     - whitespace/newline-separated node architecture tokens as reported by
#       kubectl (.status.nodeInfo.architecture), e.g. 'amd64' or 'amd64 arm64'.
#
# Output: the effective comma-separated platform list on stdout, or nothing when
# the feature is disabled. Detection outcomes:
#   - exactly one recognized arch -> that single platform (no emulation)
#   - multiple archs, or none/unknown -> the FALLBACK list
#
# The script is both sourceable (exposes compute_build_platforms) and directly
# executable (compute-build-platforms.sh MODE FALLBACK ARCHES).

set -euo pipefail

readonly DEFAULT_FALLBACK_PLATFORMS="linux/amd64,linux/arm64"
readonly MODE_PLACEHOLDER="{{TARGET_CLUSTER_ARCH_MODE}}"
readonly FALLBACK_PLACEHOLDER="{{TARGET_CLUSTER_ARCH_FALLBACK_PLATFORMS}}"

# Map a node architecture token to an OCI platform, or empty when unrecognized.
_arch_to_platform() {
    case "$1" in
        amd64 | x86_64) printf 'linux/amd64' ;;
        arm64 | aarch64) printf 'linux/arm64' ;;
        *) printf '' ;;
    esac
}

# Normalize a comma/whitespace-separated platform list: trim each entry, drop
# blanks, de-duplicate, and re-join with commas in sorted (deterministic) order.
_normalize_platforms() {
    # Turn commas into spaces, then let unquoted expansion word-split on all IFS
    # whitespace (space, tab, newline) so mixed separators collapse to one token
    # per line without relying on multi-character tr sets.
    # shellcheck disable=SC2086
    printf '%s\n' ${1//,/ } |
        sed '/^$/d' |
        sort -u |
        paste -sd, -
}

# Resolve the fallback list, applying the default when empty or a placeholder.
_resolve_fallback() {
    local fallback="$1"
    if [[ -z "$fallback" || "$fallback" == "$FALLBACK_PLACEHOLDER" ]]; then
        fallback="$DEFAULT_FALLBACK_PLATFORMS"
    fi
    _normalize_platforms "$fallback"
}

# compute_build_platforms MODE FALLBACK ARCHES -> effective platforms on stdout.
compute_build_platforms() {
    local mode="${1:-}"
    local fallback="${2:-}"
    local arches="${3:-}"

    # Feature disabled: empty or an unsubstituted placeholder. Emit nothing so
    # callers inject no platform parameter and the recipe default applies.
    if [[ -z "$mode" || "$mode" == "$MODE_PLACEHOLDER" ]]; then
        return 0
    fi

    # Explicit override: any value that looks like a platform list (contains a
    # '/') is honored verbatim, no detection.
    if [[ "$mode" == */* ]]; then
        _normalize_platforms "$mode"
        return 0
    fi

    if [[ "$mode" != "detect" ]]; then
        # Unrecognized mode keyword. Fail safe to the portable fallback rather
        # than guessing, and warn so the misconfiguration is visible.
        echo "compute-build-platforms: unrecognized mode '${mode}', using fallback platforms" >&2
        _resolve_fallback "$fallback"
        return 0
    fi

    # detect: collect the distinct recognized platforms across all nodes.
    local platforms=()
    local unknown=0
    local token platform
    for token in $arches; do
        platform=$(_arch_to_platform "$token")
        if [[ -z "$platform" ]]; then
            unknown=1
            continue
        fi
        platforms+=("$platform")
    done

    local distinct
    distinct=$(_normalize_platforms "$(printf '%s\n' "${platforms[@]:-}")")

    # Single recognized arch across the whole cluster, and nothing unrecognized:
    # build just that platform and skip emulation.
    if [[ "$unknown" -eq 0 && -n "$distinct" && "$distinct" != *,* ]]; then
        printf '%s' "$distinct"
        return 0
    fi

    # Mixed-arch, empty, or anything we could not classify confidently.
    _resolve_fallback "$fallback"
}

# Run directly when invoked as a script (not when sourced by tests).
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    compute_build_platforms "${1:-}" "${2:-}" "${3:-}"
fi
