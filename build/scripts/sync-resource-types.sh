#!/usr/bin/env bash

set -euo pipefail

# Synchronizes default resource type manifests from resource-types-contrib and
# maintains the repository's upstream pins.
#
# deploy/manifest/defaults.yaml carries two pin sections that share one entry
# shape ({name, repo, ref, tag}):
#   resourceTypes  one entry per `Radius.<Category>` namespace. The default
#                  types to ship are listed under `defaultRegistration` using
#                  <namespace>/<typeName> names, and each is fetched from the
#                  entry whose name matches its namespace. Entries sharing the
#                  same (repo, ref) are fetched once. The selected manifests and
#                  optional SVG icons are copied into every destination
#                  directory and stale managed files are pruned.
#   recipePacks    one entry per recipe pack folder published upstream (azure,
#                  kubernetes, ...). Recipe packs are not vendored into this
#                  repo, so they are only pinned - never copied.
#
# Path resolution: strip the "Radius." prefix from a defaultRegistration entry,
# then <namespace>/<typeName>/<typeName>.yaml (e.g. Radius.Compute/containers ->
# Compute/containers/containers.yaml) inside the fetched tree.
#
# Modes:
#   (default)             Copy manifests from the refs already pinned in
#                         defaults.yaml.
#   --update              Re-pin `resourceTypes`, then copy.
#   --update-recipe-packs Re-pin `recipePacks` only (nothing is copied).
#
# Both update modes select what to pin the same way, with the *_PINS variable
# winning when set:
#   * <SECTION>_PINS - a JSON array of {name, ref} objects (the
#     resource-types-contrib dispatch payload; `namespace` is accepted as an
#     alias for `name`). Each listed, *registered* entry is pinned to its ref;
#     names absent from defaults.yaml are skipped, so an upstream-only namespace
#     or recipe pack produces no change.
#   * <SECTION>_REF / <SECTION>_NAMESPACE|NAME - resolve one ref (default
#     "main") for a single entry, or for every entry when the name is empty.
# Each ref is resolved to an immutable commit SHA before pinning; when the ref
# names an upstream tag it is also recorded in the entry's `tag` field, which is
# otherwise cleared (edge channel).
#
# Environment (DEFAULTS_YAML, MANIFEST_DEST_DIRS and MANUAL_CORE_MANIFESTS are
# provided by build/resource-types.mk; defaults keep the script runnable alone):
#   DEFAULTS_YAML             Path to defaults.yaml.
#   MANIFEST_DEST_DIRS        Space-separated destination directories.
#   MANUAL_CORE_MANIFESTS     Space-separated filenames that are never pruned.
#   RESOURCE_TYPES_PINS       --update: JSON [{name, ref}, ...] to pin.
#   RESOURCE_TYPES_REF        --update: ref to resolve (default "main").
#   RESOURCE_TYPES_NAMESPACE  --update: limit to one namespace (default: all).
#   RECIPE_PACKS_PINS         --update-recipe-packs: JSON [{name, ref}, ...].
#   RECIPE_PACKS_REF          --update-recipe-packs: ref (default "main").
#   RECIPE_PACKS_NAME         --update-recipe-packs: limit to one pack.

readonly DEFAULTS_YAML="${DEFAULTS_YAML:-deploy/manifest/defaults.yaml}"
readonly MANIFEST_DEST_DIRS="${MANIFEST_DEST_DIRS:-deploy/manifest/built-in-providers/dev deploy/manifest/built-in-providers/self-hosted}"
readonly MANUAL_CORE_MANIFESTS="${MANUAL_CORE_MANIFESTS:-applications_core.yaml applications_dapr.yaml applications_datastores.yaml applications_messaging.yaml microsoft_resources.yaml radius_core.yaml}"
readonly RESOURCE_TYPES_REF="${RESOURCE_TYPES_REF:-main}"
readonly RESOURCE_TYPES_NAMESPACE="${RESOURCE_TYPES_NAMESPACE:-}"
readonly RESOURCE_TYPES_PINS="${RESOURCE_TYPES_PINS:-}"
readonly RECIPE_PACKS_REF="${RECIPE_PACKS_REF:-main}"
readonly RECIPE_PACKS_NAME="${RECIPE_PACKS_NAME:-}"
readonly RECIPE_PACKS_PINS="${RECIPE_PACKS_PINS:-}"

# YAML keys of the two pin sections in defaults.yaml.
readonly RESOURCE_TYPES_SECTION="resourceTypes"
readonly RECIPE_PACKS_SECTION="recipePacks"

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

# pin_field <section> <name> <field> -> that field from the matching pin entry
pin_field() {
    yq -r ".$1[] | select(.name == \"$2\") | .$3" "${DEFAULTS_YAML}" | head -n1
}

# validate_ref rejects values with characters outside a conservative allowlist
# so they can be interpolated into git commands safely.
validate_ref() {
    case "$1" in
        "") fail "ref must not be empty." ;;
        -*) fail "ref '$1' must not start with '-'." ;;
        *[!A-Za-z0-9._/-]*) fail "ref '$1' contains invalid characters." ;;
    esac
}

# validate_name rejects values with characters outside a conservative allowlist
# so they can be interpolated into yq expressions safely (e.g. Radius.Compute,
# azure).
validate_name() {
    case "$1" in
        "") fail "name must not be empty." ;;
        *[!A-Za-z0-9._-]*) fail "name '$1' contains invalid characters." ;;
    esac
}

# resolve_pin <repo> <ref> -> "<sha>\t<tag>". A 40-char hex value is used as-is
# with an empty tag; anything else is resolved via git ls-remote. The full
# refname ls-remote reports distinguishes a tag (recorded in <tag>) from a
# branch, and the peeled (^{}) entry wins so annotated tags resolve to their
# underlying commit.
resolve_pin() {
    local repo="$1" ref="$2" sha="" tag="" line_sha line_ref
    if printf '%s' "${ref}" | grep -Eq '^[0-9a-f]{40}$'; then
        printf '%s\t\n' "${ref}"
        return 0
    fi
    while IFS=$'\t' read -r line_sha line_ref; do
        case "${line_ref}" in
            "refs/tags/${ref}^{}")
                sha="${line_sha}"
                tag="${ref}"
                break
                ;;
            "refs/tags/${ref}")
                sha="${sha:-${line_sha}}"
                tag="${ref}"
                ;;
            *) sha="${sha:-${line_sha}}" ;;
        esac
    done < <(git ls-remote "https://${repo}.git" "${ref}" "${ref}^{}")
    [ -n "${sha}" ] || fail "Could not resolve ref '${ref}' in ${repo}."
    printf '%s\t%s\n' "${sha}" "${tag}"
}

# write_pin <section> <name> <sha> <tag> records a resolved pin in defaults.yaml.
write_pin() {
    yq -i "(.$1[] | select(.name == \"$2\")) |= (.ref = \"$3\" | .tag = \"$4\")" "${DEFAULTS_YAML}"
    if [ -n "$4" ]; then
        echo "  Pinned $2 -> $3 ($4)"
    else
        echo "  Pinned $2 -> $3"
    fi
}

# pin_one <section> <name> <ref> resolves <ref> against the entry's repo and
# records the resulting commit SHA (and tag, when the ref named one).
pin_one() {
    local section="$1" name="$2" ref="$3" repo sha tag
    repo="$(pin_field "${section}" "${name}" repo)"
    { [ -n "${repo}" ] && [ "${repo}" != "null" ]; } || fail "repo is not set for '${name}' under ${section} in ${DEFAULTS_YAML}."
    echo "Resolving '${ref}' for ${name} in ${repo}..."
    IFS=$'\t' read -r sha tag < <(resolve_pin "${repo}" "${ref}")
    write_pin "${section}" "${name}" "${sha}" "${tag}"
}

# entry_names <section> -> the validated names of every entry in that section.
entry_names() {
    local section="$1" names name
    if ! names="$(
        yq -e -r "
            select(
                (.${section} | type) == \"!!seq\" and
                (.${section} | length) > 0 and
                ([.${section}[] | select(
                    type != \"!!map\" or
                    has(\"name\") == false or
                    (.name | type) != \"!!str\" or
                    (.name | test(\"^[A-Za-z0-9._-]+$\")) == false
                )] | length == 0)
            ) |
            .${section}[].name
        " "${DEFAULTS_YAML}"
    )"; then
        fail "${section} must contain valid name strings in ${DEFAULTS_YAML}."
    fi
    while IFS= read -r name; do
        validate_name "${name}"
    done <<< "${names}"
    printf '%s\n' "${names}"
}

# used_namespaces -> the distinct namespaces referenced by defaultRegistration
used_namespaces() {
    local entry
    for entry in $(yq -r '.defaultRegistration[]' "${DEFAULTS_YAML}"); do
        namespace_of "${entry}"
        echo
    done | sort -u
}

# update_section <section> <ref> <target> resolves and pins every entry in the
# section (or just <target> when set) to an immutable commit SHA.
update_section() {
    local section="$1" ref="$2" target="$3" names name matched=false
    validate_ref "${ref}"
    if [ -n "${target}" ]; then
        validate_name "${target}"
    fi
    names="$(entry_names "${section}")"

    while IFS= read -r name; do
        if [ -n "${target}" ] && [ "${name}" != "${target}" ]; then
            continue
        fi
        matched=true
        pin_one "${section}" "${name}" "${ref}"
    done <<< "${names}"
    [ "${matched}" = true ] || fail "'${target}' not found under ${section} in ${DEFAULTS_YAML}."
}

# apply_pins <section> <pins_json> <var_name> pins the entries listed in a JSON
# array of {name, ref} objects (e.g. the resource-types-contrib dispatch
# payload, where the key is spelled `namespace`) to their resolved commit SHAs.
# Names absent from the section are skipped, so an upstream-only namespace or
# recipe pack produces no change - Radius pins only what it consumes. yq parses
# the JSON, so no extra tooling is required.
apply_pins() {
    local section="$1" pins_json="$2" var_name="$3"
    local parsed_pins name ref repo applied=0
    if ! parsed_pins="$(
        printf '%s' "${pins_json}" |
            yq -p=json -e -r '
                select(
                    type == "!!seq" and
                    length > 0 and
                    ([.[] | select(
                        type != "!!map" or
                        (has("name") == false and has("namespace") == false) or
                        has("ref") == false or
                        ((.name // .namespace) | type) != "!!str" or
                        (.ref | type) != "!!str"
                    )] | length == 0)
                ) |
                .[] |
                (.name // .namespace) + "|" + .ref
            '
    )"; then
        fail "${var_name} must be a valid JSON array of {name, ref} objects."
    fi

    while IFS='|' read -r name ref; do
        [ -n "${name}" ] || continue
        validate_name "${name}"
        validate_ref "${ref}"
        repo="$(pin_field "${section}" "${name}" repo)"
        if [ -z "${repo}" ] || [ "${repo}" = "null" ]; then
            echo "  Skipping ${name}: not registered under ${section} in ${DEFAULTS_YAML}"
            continue
        fi
        pin_one "${section}" "${name}" "${ref}"
        applied=$((applied + 1))
    done <<< "${parsed_pins}"
    [ "${applied}" -gt 0 ] || echo "No registered entries in ${var_name}; nothing re-pinned."
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
# type belonging to that resourceTypes entry into all destination directories.
copy_manifests() {
    local tmp_root pairs_file i=0 repo ref dir entry ns rel type src src_icon dest
    tmp_root="$(mktemp -d)"
    # shellcheck disable=SC2064
    trap "rm -rf '${tmp_root}'" EXIT

    pairs_file="${tmp_root}/pairs"
    : > "${pairs_file}"
    for ns in $(used_namespaces); do
        repo="$(pin_field "${RESOURCE_TYPES_SECTION}" "${ns}" repo)"
        ref="$(pin_field "${RESOURCE_TYPES_SECTION}" "${ns}" ref)"
        { [ -n "${repo}" ] && [ "${repo}" != "null" ]; } || fail "repo is not set for namespace '${ns}' under ${RESOURCE_TYPES_SECTION} in ${DEFAULTS_YAML}."
        { [ -n "${ref}" ] && [ "${ref}" != "null" ]; } || fail "ref is not set for namespace '${ns}' under ${RESOURCE_TYPES_SECTION} in ${DEFAULTS_YAML}."
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
            [ "$(pin_field "${RESOURCE_TYPES_SECTION}" "${ns}" repo)|$(pin_field "${RESOURCE_TYPES_SECTION}" "${ns}" ref)" = "${repo}|${ref}" ] || continue
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
        --update-recipe-packs) mode="update-recipe-packs" ;;
        "") ;;
        *) fail "Unknown argument: $1 (expected --update, --update-recipe-packs, or no arguments)." ;;
    esac

    require_tools
    [ -f "${DEFAULTS_YAML}" ] || fail "defaults file not found: ${DEFAULTS_YAML}"

    # Recipe packs are pinned but never vendored, so this mode does not copy.
    if [ "${mode}" = update-recipe-packs ]; then
        if [ -n "${RECIPE_PACKS_PINS}" ]; then
            apply_pins "${RECIPE_PACKS_SECTION}" "${RECIPE_PACKS_PINS}" RECIPE_PACKS_PINS
        else
            update_section "${RECIPE_PACKS_SECTION}" "${RECIPE_PACKS_REF}" "${RECIPE_PACKS_NAME}"
        fi
        echo "Done. Review and commit the updated ${DEFAULTS_YAML}."
        return 0
    fi

    if [ "${mode}" = update ]; then
        if [ -n "${RESOURCE_TYPES_PINS}" ]; then
            apply_pins "${RESOURCE_TYPES_SECTION}" "${RESOURCE_TYPES_PINS}" RESOURCE_TYPES_PINS
        else
            update_section "${RESOURCE_TYPES_SECTION}" "${RESOURCE_TYPES_REF}" "${RESOURCE_TYPES_NAMESPACE}"
        fi
    fi

    echo "Syncing default resource types from resource-types-contrib..."
    copy_manifests
    prune_stale
    echo "Done. Review and commit the updated files."
}

main "$@"
