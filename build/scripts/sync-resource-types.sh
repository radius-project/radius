#!/usr/bin/env bash

set -euo pipefail

# Synchronizes default resource type manifests from resource-types-contrib.
#
# deploy/manifest/defaults.yaml pins, per namespace, an upstream source
# (repo + ref) under `sources`, and lists the default resource types to ship
# under `defaultRegistration` using <namespace>/<typeName> names. Each default
# type is fetched from the source whose namespace it matches; entries that share
# the same (repo, ref) are fetched once. The selected manifests and optional SVG
# icons are copied into every destination directory and stale managed files
# are pruned.
#
# Path resolution: strip the "Radius." prefix from an entry, then
#   <namespace>/<typeName>/<typeName>.yaml   (e.g. Radius.Compute/containers ->
#   Compute/containers/containers.yaml) inside the fetched tree.
#
# Modes:
#   (default)   Copy manifests from the refs already pinned in defaults.yaml.
#   --update    Re-pin one or more namespaces, then copy. Two ways to select
#               what to pin (RESOURCE_TYPES_PINS wins when set):
#                 * RESOURCE_TYPES_PINS  - a JSON array of {namespace, ref}
#                   (the resource-types-contrib dispatch payload). Each listed,
#                   *registered* namespace is pinned to its ref; namespaces not
#                   in defaults.yaml `sources` are skipped.
#                 * RESOURCE_TYPES_REF / RESOURCE_TYPES_NAMESPACE - resolve one
#                   ref (default "main") for a single namespace, or all when
#                   RESOURCE_TYPES_NAMESPACE is empty.
#               Each ref is resolved to an immutable commit SHA before pinning.
#
# Environment (DEFAULTS_YAML, MANIFEST_DEST_DIRS and MANUAL_CORE_MANIFESTS are
# provided by build/resource-types.mk; defaults keep the script runnable alone):
#   DEFAULTS_YAML             Path to defaults.yaml.
#   MANIFEST_DEST_DIRS        Space-separated destination directories.
#   MANUAL_CORE_MANIFESTS     Space-separated filenames that are never pruned.
#   RESOURCE_TYPES_PINS       --update: JSON [{namespace, ref}, ...] to pin.
#   RESOURCE_TYPES_REF        --update: ref to resolve (default "main").
#   RESOURCE_TYPES_NAMESPACE  --update: limit to one namespace (default: all).

readonly DEFAULTS_YAML="${DEFAULTS_YAML:-deploy/manifest/defaults.yaml}"
readonly MANIFEST_DEST_DIRS="${MANIFEST_DEST_DIRS:-deploy/manifest/built-in-providers/dev deploy/manifest/built-in-providers/self-hosted}"
readonly MANUAL_CORE_MANIFESTS="${MANUAL_CORE_MANIFESTS:-applications_core.yaml applications_dapr.yaml applications_datastores.yaml applications_messaging.yaml microsoft_resources.yaml radius_core.yaml}"
readonly RESOURCE_TYPES_REF="${RESOURCE_TYPES_REF:-main}"
readonly RESOURCE_TYPES_NAMESPACE="${RESOURCE_TYPES_NAMESPACE:-}"
readonly RESOURCE_TYPES_PINS="${RESOURCE_TYPES_PINS:-}"

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

require_tools() {
    command -v yq > /dev/null 2>&1 || fail "yq is required but not found. Install via: make install-yq"
    command -v git > /dev/null 2>&1 || fail "git is required but not found."
}

# namespace_of <namespace>/<typeName> -> <namespace>
namespace_of() { printf '%s' "${1%%/*}"; }

# source_field <namespace> <field> -> that field from the matching source entry
source_field() {
    yq -r ".sources[] | select(.namespace == \"$1\") | .$2" "${DEFAULTS_YAML}" | head -n1
}

# validate_ref rejects values with characters outside a conservative allowlist
# so they can be interpolated into git commands safely.
validate_ref() {
    case "$1" in
        "") fail "ref must not be empty." ;;
        *[!A-Za-z0-9._/-]*) fail "ref '$1' contains invalid characters." ;;
    esac
}

# validate_namespace rejects values with characters outside a conservative
# allowlist so they can be interpolated into yq expressions safely (e.g.
# Radius.Compute).
validate_namespace() {
    case "$1" in
        "") fail "namespace must not be empty." ;;
        *[!A-Za-z0-9._-]*) fail "namespace '$1' contains invalid characters." ;;
    esac
}

# resolve_ref <repo> <ref> -> an immutable commit SHA. A 40-char hex value is
# used as-is; a branch/tag is resolved via git ls-remote, preferring the peeled
# (^{}) commit so annotated tags resolve to their underlying commit.
resolve_ref() {
    local repo="$1" ref="$2" sha=""
    if printf '%s' "${ref}" | grep -Eq '^[0-9a-f]{40}$'; then
        printf '%s' "${ref}"
        return 0
    fi
    sha="$(git ls-remote "https://${repo}.git" "${ref}^{}" | head -n1 | cut -f1)"
    if [ -z "${sha}" ]; then
        sha="$(git ls-remote "https://${repo}.git" "${ref}" | head -n1 | cut -f1)"
    fi
    [ -n "${sha}" ] || fail "Could not resolve ref '${ref}' in ${repo}."
    printf '%s' "${sha}"
}

# used_namespaces -> the distinct namespaces referenced by defaultRegistration
used_namespaces() {
    local entry
    for entry in $(yq -r '.defaultRegistration[]' "${DEFAULTS_YAML}"); do
        namespace_of "${entry}"
        echo
    done | sort -u
}

# update_sources resolves and pins each source's ref (all namespaces, or just
# RESOURCE_TYPES_NAMESPACE when set) to an immutable commit SHA.
update_sources() {
    validate_ref "${RESOURCE_TYPES_REF}"
    local target="${RESOURCE_TYPES_NAMESPACE}" matched=false ns repo sha
    for ns in $(yq -r '.sources[].namespace' "${DEFAULTS_YAML}"); do
        if [ -n "${target}" ] && [ "${ns}" != "${target}" ]; then
            continue
        fi
        matched=true
        repo="$(source_field "${ns}" repo)"
        { [ -n "${repo}" ] && [ "${repo}" != "null" ]; } || fail "source.repo is not set for namespace '${ns}' in ${DEFAULTS_YAML}."
        echo "Resolving '${RESOURCE_TYPES_REF}' for ${ns} in ${repo}..."
        sha="$(resolve_ref "${repo}" "${RESOURCE_TYPES_REF}")"
        yq -i "(.sources[] | select(.namespace == \"${ns}\") | .ref) = \"${sha}\"" "${DEFAULTS_YAML}"
        echo "  Pinned ${ns} -> ${sha}"
    done
    [ "${matched}" = true ] || fail "namespace '${target}' not found under sources in ${DEFAULTS_YAML}."
}

# apply_pins pins the namespaces listed in RESOURCE_TYPES_PINS (a JSON array of
# {namespace, ref}, e.g. the resource-types-contrib dispatch payload) to their
# resolved commit SHAs. Namespaces absent from defaults.yaml `sources` are
# skipped, so an upstream-only namespace produces no change (Radius vendors only
# what it registers). yq parses the JSON, so no extra tooling is required.
apply_pins() {
    local ns ref repo sha applied=0
    while read -r ns ref; do
        [ -n "${ns}" ] || continue
        validate_namespace "${ns}"
        validate_ref "${ref}"
        repo="$(source_field "${ns}" repo)"
        if [ -z "${repo}" ] || [ "${repo}" = "null" ]; then
            echo "  Skipping ${ns}: not registered in ${DEFAULTS_YAML} sources"
            continue
        fi
        echo "Resolving '${ref}' for ${ns} in ${repo}..."
        sha="$(resolve_ref "${repo}" "${ref}")"
        yq -i "(.sources[] | select(.namespace == \"${ns}\") | .ref) = \"${sha}\"" "${DEFAULTS_YAML}"
        echo "  Pinned ${ns} -> ${sha}"
        applied=$((applied + 1))
    done < <(printf '%s' "${RESOURCE_TYPES_PINS}" | yq -p=json '.[] | .namespace + " " + .ref')
    [ "${applied}" -gt 0 ] || echo "No registered namespaces in RESOURCE_TYPES_PINS; nothing re-pinned."
}

# fetch_ref <repo> <ref> <dir> shallow-fetches the ref into an empty dir.
fetch_ref() {
    local repo="$1" ref="$2" dir="$3"
    git init -q "${dir}"
    git -C "${dir}" remote add origin "https://${repo}.git"
    if ! git -C "${dir}" fetch -q --depth 1 origin "${ref}"; then
        fail "Failed to fetch ref '${ref}' from ${repo}. It must be a full commit SHA, tag, or branch reachable upstream."
    fi
    git -C "${dir}" checkout -q FETCH_HEAD
}

# copy_manifests fetches each distinct (repo, ref) once and copies every default
# type belonging to that source into all destination directories.
copy_manifests() {
    local tmp_root pairs_file i=0 repo ref dir entry ns rel type src src_icon dest
    tmp_root="$(mktemp -d)"
    # shellcheck disable=SC2064
    trap "rm -rf '${tmp_root}'" EXIT

    pairs_file="${tmp_root}/pairs"
    : > "${pairs_file}"
    for ns in $(used_namespaces); do
        repo="$(source_field "${ns}" repo)"
        ref="$(source_field "${ns}" ref)"
        { [ -n "${repo}" ] && [ "${repo}" != "null" ]; } || fail "source.repo is not set for namespace '${ns}' in ${DEFAULTS_YAML}."
        { [ -n "${ref}" ] && [ "${ref}" != "null" ]; } || fail "source.ref is not set for namespace '${ns}' in ${DEFAULTS_YAML}."
        printf '%s|%s\n' "${repo}" "${ref}" >> "${pairs_file}"
    done
    sort -u "${pairs_file}" -o "${pairs_file}"

    while IFS='|' read -r repo ref; do
        [ -n "${repo}" ] || continue
        dir="${tmp_root}/src_${i}"
        i=$((i + 1))
        echo "  Source: ${repo} @ ${ref}"
        fetch_ref "${repo}" "${ref}" "${dir}"
        for entry in $(yq -r '.defaultRegistration[]' "${DEFAULTS_YAML}"); do
            ns="$(namespace_of "${entry}")"
            [ "$(source_field "${ns}" repo)|$(source_field "${ns}" ref)" = "${repo}|${ref}" ] || continue
            rel="${entry#Radius.}"
            type="${rel##*/}"
            src="${dir}/${rel}/${type}.yaml"
            src_icon="${dir}/${rel}/${type}.svg"
            [ -f "${src}" ] || fail "File not found: ${rel}/${type}.yaml (from entry '${entry}'). Verify the entry and the pinned ref."
            for dest in ${MANIFEST_DEST_DIRS}; do
                cp "${src}" "${dest}/${type}.yaml"
                if [ -f "${src_icon}" ]; then
                    cp "${src_icon}" "${dest}/${type}.svg"
                else
                    rm -f "${dest}/${type}.svg"
                fi
            done
            if [ -f "${src_icon}" ]; then
                echo "  Copied ${entry} (with icon)"
            else
                echo "  Copied ${entry}"
            fi
        done
    done < "${pairs_file}"
}

# prune_stale removes managed manifests and icons that are no longer in
# defaultRegistration (manual core manifests are always preserved).
prune_stale() {
    local expected="" entry rel type dest file base mc ef is_manual is_expected
    for entry in $(yq -r '.defaultRegistration[]' "${DEFAULTS_YAML}"); do
        rel="${entry#Radius.}"
        type="${rel##*/}"
        expected="${expected} ${type}.yaml ${type}.svg"
    done
    for dest in ${MANIFEST_DEST_DIRS}; do
        for file in "${dest}"/*.yaml "${dest}"/*.svg; do
            [ -e "${file}" ] || continue
            base="$(basename "${file}")"
            is_manual=false
            for mc in ${MANUAL_CORE_MANIFESTS}; do
                if [ "${base}" = "${mc}" ]; then
                    is_manual=true
                    break
                fi
            done
            [ "${is_manual}" = true ] && continue
            is_expected=false
            for ef in ${expected}; do
                if [ "${base}" = "${ef}" ]; then
                    is_expected=true
                    break
                fi
            done
            if [ "${is_expected}" = false ]; then
                echo "  Removing stale file: ${file}"
                rm "${file}"
            fi
        done
    done
}

main() {
    local mode="sync"
    case "${1:-}" in
        --update) mode="update" ;;
        "") ;;
        *) fail "Unknown argument: $1 (expected --update or no arguments)." ;;
    esac

    require_tools
    [ -f "${DEFAULTS_YAML}" ] || fail "defaults file not found: ${DEFAULTS_YAML}"

    if [ "${mode}" = update ]; then
        if [ -n "${RESOURCE_TYPES_PINS}" ]; then
            apply_pins
        else
            update_sources
        fi
    fi

    echo "Syncing default resource types from resource-types-contrib..."
    copy_manifests
    prune_stale
    echo "Done. Review and commit the updated files."
}

main "$@"
