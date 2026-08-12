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
#   (default)             Validate all pins and copy resource type manifests.
#   --update              Re-pin `resourceTypes`, then copy.
#   --update-recipe-packs Re-pin `recipePacks` only (nothing is copied).
#   --update-all          Atomically re-pin both sections, then copy manifests.
#
# Both update modes select what to pin the same way, with the *_PINS variable
# winning when set:
#   * <SECTION>_PINS - a JSON array of {name, ref} objects (the
#     resource-types-contrib dispatch payload; `namespace` is accepted as an
#     alias for `name`). Each listed, *registered* entry is pinned to its ref;
#     names absent from defaults.yaml are skipped, so an upstream-only namespace
#     or recipe pack produces no change.
#   * <SECTION>_REF / <SECTION>_NAMESPACE|NAME - request one ref (default
#     "main") for a single entry, or for every entry when the name is empty.
# Selection is stable-first per entry. An explicit stable unit tag is retained;
# otherwise the latest stable unit tag wins. The requested edge ref (`main` or a
# full commit SHA) is used only when that unit has no stable release. Prerelease
# tags are never selected. The chosen ref resolves to an immutable commit SHA in
# `ref`; `tag` records the stable release tag or is empty for the edge fallback.
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

# Resolution results are set by load_stable_tags and load_resolved_pin. Keeping
# them in the current shell preserves command failures and the commit cache.
STABLE_SCOPED_TAG=""
STABLE_GLOBAL_TAG=""
RESOLVED_SHA=""
RESOLVED_TAG=""
declare -A VERIFIED_COMMITS=()

fail() {
    echo "ERROR: $*" >&2
    exit 1
}

require_tools() {
    command -v yq >/dev/null 2>&1 || fail "yq is required but not found. Install via: make install-yq"
    command -v git >/dev/null 2>&1 || fail "git is required but not found."
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

# stable_tag_prefix <section> <name> -> the unit-scoped release tag prefix.
stable_tag_prefix() {
    case "$1" in
        "${RESOURCE_TYPES_SECTION}") printf '%s' "$2" ;;
        "${RECIPE_PACKS_SECTION}") printf 'recipe-pack/%s' "$2" ;;
        *) fail "unknown pin section '$1'." ;;
    esac
}

# is_stable_version <version> accepts a stable SemVer core without a leading v.
is_stable_version() {
    [[ "$1" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]
}

# is_scoped_stable_tag <section> <name> <tag> accepts the unit's stable tag.
is_scoped_stable_tag() {
    local prefix version
    prefix="$(stable_tag_prefix "$1" "$2")"
    case "$3" in
        "${prefix}/v"*) version="${3#"${prefix}/v"}" ;;
        *) return 1 ;;
    esac
    is_stable_version "${version}"
}

# is_global_stable_tag <tag> accepts a legacy repository-wide stable tag.
is_global_stable_tag() {
    [[ "$1" == v* ]] && is_stable_version "${1#v}"
}

# is_stable_tag <section> <name> <tag> accepts either stable tag form.
is_stable_tag() {
    is_scoped_stable_tag "$1" "$2" "$3" || is_global_stable_tag "$3"
}

# is_edge_ref <ref> accepts the moving edge branch or an immutable commit from
# that channel. Other branches and prerelease tags are not release fallbacks.
is_edge_ref() {
    [[ "$1" == "main" || "$1" =~ ^[0-9a-fA-F]{40}$ ]]
}

# load_stable_tags <repo> <section> <name> sets the latest scoped and global
# stable tags. Callers decide precedence; lookup failures propagate unchanged.
load_stable_tags() {
    local repo="$1" section="$2" name="$3" prefix refs
    local line_sha line_ref tag scoped_tags="" global_tags=""
    STABLE_SCOPED_TAG=""
    STABLE_GLOBAL_TAG=""
    prefix="$(stable_tag_prefix "${section}" "${name}")"
    if ! refs="$(
        git ls-remote --tags --refs "https://${repo}.git" \
            "refs/tags/${prefix}/v*" \
            "refs/tags/v*"
    )"; then
        echo "ERROR: Could not list stable releases for '${name}' in ${repo}." >&2
        return 1
    fi
    while IFS=$'\t' read -r line_sha line_ref; do
        [[ -n "${line_sha}" && -n "${line_ref}" ]] || continue
        tag="${line_ref#refs/tags/}"
        if is_scoped_stable_tag "${section}" "${name}" "${tag}"; then
            scoped_tags+="${tag}"$'\n'
        elif is_global_stable_tag "${tag}"; then
            global_tags+="${tag}"$'\n'
        fi
    done <<<"${refs}"
    if [[ -n "${scoped_tags}" ]]; then
        STABLE_SCOPED_TAG="$(printf '%s' "${scoped_tags}" | sort -V | tail -n1)"
    fi
    if [[ -n "${global_tags}" ]]; then
        STABLE_GLOBAL_TAG="$(printf '%s' "${global_tags}" | sort -V | tail -n1)"
    fi
}

# latest_stable_tag <repo> <section> <name> prints the preferred stable tag.
latest_stable_tag() {
    load_stable_tags "$@" || return 1
    printf '%s' "${STABLE_SCOPED_TAG:-${STABLE_GLOBAL_TAG}}"
}

# preferred_ref <repo> <section> <name> <requested_ref> chooses a stable
# release whenever one exists. An explicit stable tag is retained to support
# rollback; all other refs are treated as edge candidates and are used only for
# units that have no stable release.
preferred_ref() {
    local repo="$1" section="$2" name="$3" requested_ref="$4" stable_tag
    if is_scoped_stable_tag "${section}" "${name}" "${requested_ref}"; then
        printf '%s' "${requested_ref}"
        return 0
    fi
    load_stable_tags "${repo}" "${section}" "${name}" || return 1
    if [[ -n "${STABLE_SCOPED_TAG}" ]]; then
        stable_tag="${STABLE_SCOPED_TAG}"
    elif is_global_stable_tag "${requested_ref}"; then
        stable_tag="${requested_ref}"
    else
        stable_tag="${STABLE_GLOBAL_TAG}"
    fi
    if [[ -n "${stable_tag}" ]]; then
        echo "  Using stable release ${stable_tag} instead of edge ref ${requested_ref}." >&2
        printf '%s' "${stable_tag}"
    else
        is_edge_ref "${requested_ref}" ||
            fail "No stable release exists for ${name}; edge ref must be 'main' or a full commit SHA, not '${requested_ref}'."
        echo "  No stable release exists for ${name}; using edge ref ${requested_ref}." >&2
        printf '%s' "${requested_ref}"
    fi
}

# verify_commit_exists <repo> <sha> fetches a commit once to prove it exists in
# the upstream repository. This matters for recipe packs, which are not copied.
verify_commit_exists() {
    local repo="$1" sha="${2,,}" key tmp_root resolved
    key="${repo}@${sha}"
    [[ -n "${VERIFIED_COMMITS[${key}]:-}" ]] && return 0
    tmp_root="$(mktemp -d)"
    git init -q "${tmp_root}"
    git -C "${tmp_root}" remote add origin "https://${repo}.git"
    if ! git -C "${tmp_root}" fetch -q --depth 1 origin "${sha}"; then
        rm -rf "${tmp_root}"
        echo "ERROR: Commit '${sha}' does not exist in ${repo}." >&2
        return 1
    fi
    resolved="$(git -C "${tmp_root}" rev-parse FETCH_HEAD)"
    rm -rf "${tmp_root}"
    if [[ "${resolved,,}" != "${sha}" ]]; then
        echo "ERROR: Commit '${sha}' resolved to '${resolved}' in ${repo}." >&2
        return 1
    fi
    VERIFIED_COMMITS["${key}"]=true
}

# load_resolved_pin <repo> <ref> sets RESOLVED_SHA and RESOLVED_TAG. Exact
# head/tag refnames avoid branch/tag ambiguity; annotated tags use peeled SHAs.
load_resolved_pin() {
    local repo="$1" ref="$2" refs line_sha line_ref
    RESOLVED_SHA=""
    RESOLVED_TAG=""
    # git accepts uppercase SHAs but always prints lowercase; normalize so pins
    # written from either form stay comparable.
    if printf '%s' "${ref}" | grep -Eqi '^[0-9a-f]{40}$'; then
        RESOLVED_SHA="$(printf '%s' "${ref}" | tr '[:upper:]' '[:lower:]')"
        verify_commit_exists "${repo}" "${RESOLVED_SHA}" || return 1
        return 0
    fi
    if ! refs="$(
        git ls-remote "https://${repo}.git" \
            "refs/heads/${ref}" \
            "refs/tags/${ref}" \
            "refs/tags/${ref}^{}"
    )"; then
        fail "Could not resolve ref '${ref}' in ${repo}."
    fi
    while IFS=$'\t' read -r line_sha line_ref; do
        case "${line_ref}" in
            "refs/tags/${ref}^{}")
                RESOLVED_SHA="${line_sha}"
                RESOLVED_TAG="${ref}"
                break
                ;;
            "refs/tags/${ref}")
                RESOLVED_SHA="${line_sha}"
                RESOLVED_TAG="${ref}"
                ;;
            "refs/heads/${ref}")
                [[ -n "${RESOLVED_TAG}" ]] || RESOLVED_SHA="${line_sha}"
                ;;
        esac
    done <<<"${refs}"
    if [[ -z "${RESOLVED_SHA}" ]]; then
        echo "ERROR: Could not resolve ref '${ref}' in ${repo}." >&2
        return 1
    fi
}

# resolve_pin <repo> <ref> prints load_resolved_pin's result for callers that
# need a value instead of the state-setting interface.
resolve_pin() {
    load_resolved_pin "$@" || return 1
    printf '%s\t%s\n' "${RESOLVED_SHA}" "${RESOLVED_TAG}"
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
    local section="$1" name="$2" ref="$3" repo selected_ref tag=""
    repo="$(pin_field "${section}" "${name}" repo)"
    { [ -n "${repo}" ] && [ "${repo}" != "null" ]; } || fail "repo is not set for '${name}' under ${section} in ${DEFAULTS_YAML}."
    if ! selected_ref="$(preferred_ref "${repo}" "${section}" "${name}" "${ref}")"; then
        return 1
    fi
    echo "Resolving '${selected_ref}' for ${name} in ${repo}..."
    load_resolved_pin "${repo}" "${selected_ref}" || return 1
    if is_stable_tag "${section}" "${name}" "${selected_ref}"; then
        [[ "${RESOLVED_TAG}" == "${selected_ref}" ]] ||
            fail "Stable release tag '${selected_ref}' does not exist in ${repo}."
        tag="${selected_ref}"
    fi
    write_pin "${section}" "${name}" "${RESOLVED_SHA}" "${tag}"
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
    done <<<"${names}"
    printf '%s\n' "${names}"
}

# promote_edge_pins <section> upgrades every edge entry in a section when a
# stable release now exists. This closes races between independently published
# unit releases and targeted repository_dispatch events.
promote_edge_pins() {
    local section="$1" names name repo tag stable_tag
    names="$(entry_names "${section}")"
    while IFS= read -r name; do
        tag="$(pin_field "${section}" "${name}" tag)"
        [[ "${tag}" == "null" ]] && tag=""
        [[ -z "${tag}" ]] || continue
        repo="$(pin_field "${section}" "${name}" repo)"
        if ! stable_tag="$(latest_stable_tag "${repo}" "${section}" "${name}")"; then
            return 1
        fi
        if [[ -n "${stable_tag}" ]]; then
            pin_one "${section}" "${name}" "${stable_tag}"
        fi
    done <<<"${names}"
}

# validate_section_pins <section> enforces the catalog policy: each entry must
# point to a stable unit release, except when that unit has no stable release,
# in which case an immutable edge commit is allowed.
validate_section_pins() {
    local section="$1" names name repo ref tag stable_tag
    names="$(entry_names "${section}")"
    while IFS= read -r name; do
        repo="$(pin_field "${section}" "${name}" repo)"
        ref="$(pin_field "${section}" "${name}" ref)"
        tag="$(pin_field "${section}" "${name}" tag)"
        [[ "${tag}" == "null" ]] && tag=""
        [[ "${ref}" =~ ^[0-9a-fA-F]{40}$ ]] ||
            fail "${section} entry '${name}' must pin a full commit SHA."
        if [[ -n "${tag}" ]]; then
            is_stable_tag "${section}" "${name}" "${tag}" ||
                fail "${section} entry '${name}' has non-stable tag '${tag}'."
            if is_global_stable_tag "${tag}"; then
                load_stable_tags "${repo}" "${section}" "${name}" || return 1
                [[ -z "${STABLE_SCOPED_TAG}" ]] ||
                    fail "${section} entry '${name}' uses global release ${tag}, but scoped release ${STABLE_SCOPED_TAG} exists."
            fi
            load_resolved_pin "${repo}" "${tag}" || return 1
            [[ "${RESOLVED_TAG}" == "${tag}" ]] ||
                fail "${section} entry '${name}' tag '${tag}' does not exist in ${repo}."
            [[ "${RESOLVED_SHA,,}" == "${ref,,}" ]] ||
                fail "${section} entry '${name}' pins ${ref}, but ${tag} resolves to ${RESOLVED_SHA}."
            continue
        fi
        if ! stable_tag="$(latest_stable_tag "${repo}" "${section}" "${name}")"; then
            return 1
        fi
        [[ -z "${stable_tag}" ]] ||
            fail "${section} entry '${name}' uses edge ref ${ref}, but stable release ${stable_tag} exists."
        load_resolved_pin "${repo}" "${ref}" || return 1
        [[ "${RESOLVED_SHA,,}" == "${ref,,}" ]] ||
            fail "${section} entry '${name}' edge ref ${ref} resolved to ${RESOLVED_SHA}."
    done <<<"${names}"
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
    done <<<"${names}"
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
    done <<<"${parsed_pins}"
    [ "${applied}" -gt 0 ] || echo "No registered entries in ${var_name}; nothing re-pinned."
}

has_pins() {
    local trimmed
    trimmed="$(printf '%s' "$1" | tr -d '[:space:]')"
    [[ -n "${trimmed}" && "${trimmed}" != "[]" ]]
}

# update_all_sections applies both pin sections before validating either one.
# Dispatch payloads may contain one or both sections; without payloads, every
# entry is refreshed using stable-first selection and its configured edge ref.
update_all_sections() {
    if has_pins "${RESOURCE_TYPES_PINS}" || has_pins "${RECIPE_PACKS_PINS}"; then
        if has_pins "${RESOURCE_TYPES_PINS}"; then
            apply_pins "${RESOURCE_TYPES_SECTION}" "${RESOURCE_TYPES_PINS}" RESOURCE_TYPES_PINS
        fi
        if has_pins "${RECIPE_PACKS_PINS}"; then
            apply_pins "${RECIPE_PACKS_SECTION}" "${RECIPE_PACKS_PINS}" RECIPE_PACKS_PINS
        fi
    else
        update_section "${RESOURCE_TYPES_SECTION}" "${RESOURCE_TYPES_REF}" "${RESOURCE_TYPES_NAMESPACE}"
        update_section "${RECIPE_PACKS_SECTION}" "${RECIPE_PACKS_REF}" "${RECIPE_PACKS_NAME}"
    fi

    promote_edge_pins "${RESOURCE_TYPES_SECTION}"
    promote_edge_pins "${RECIPE_PACKS_SECTION}"
    validate_section_pins "${RESOURCE_TYPES_SECTION}"
    validate_section_pins "${RECIPE_PACKS_SECTION}"
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
    local pack pack_repo pack_ref
    tmp_root="$(mktemp -d)"
    # shellcheck disable=SC2064
    trap "rm -rf '${tmp_root}'" EXIT

    pairs_file="${tmp_root}/pairs"
    : >"${pairs_file}"
    for ns in $(used_namespaces); do
        repo="$(pin_field "${RESOURCE_TYPES_SECTION}" "${ns}" repo)"
        ref="$(pin_field "${RESOURCE_TYPES_SECTION}" "${ns}" ref)"
        { [ -n "${repo}" ] && [ "${repo}" != "null" ]; } || fail "repo is not set for namespace '${ns}' under ${RESOURCE_TYPES_SECTION} in ${DEFAULTS_YAML}."
        { [ -n "${ref}" ] && [ "${ref}" != "null" ]; } || fail "ref is not set for namespace '${ns}' under ${RESOURCE_TYPES_SECTION} in ${DEFAULTS_YAML}."
        printf '%s|%s\n' "${repo}" "${ref}" >>"${pairs_file}"
    done
    for pack in $(entry_names "${RECIPE_PACKS_SECTION}"); do
        repo="$(pin_field "${RECIPE_PACKS_SECTION}" "${pack}" repo)"
        ref="$(pin_field "${RECIPE_PACKS_SECTION}" "${pack}" ref)"
        printf '%s|%s\n' "${repo}" "${ref}" >>"${pairs_file}"
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
        for pack in $(entry_names "${RECIPE_PACKS_SECTION}"); do
            pack_repo="$(pin_field "${RECIPE_PACKS_SECTION}" "${pack}" repo)"
            pack_ref="$(pin_field "${RECIPE_PACKS_SECTION}" "${pack}" ref)"
            [[ "${pack_repo}|${pack_ref}" == "${repo}|${ref}" ]] || continue
            [[ -d "${dir}/recipe-packs/${pack}" ]] ||
                fail "Recipe pack directory not found: recipe-packs/${pack} at ${repo}@${ref}."
            echo "  Verified recipe pack ${pack}"
        done
    done <"${pairs_file}"
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
        --update-all) mode="update-all" ;;
        "") ;;
        *) fail "Unknown argument: $1 (expected --update, --update-recipe-packs, --update-all, or no arguments)." ;;
    esac

    require_tools
    [ -f "${DEFAULTS_YAML}" ] || fail "defaults file not found: ${DEFAULTS_YAML}"

    if [[ "${mode}" == update-all ]]; then
        update_all_sections
        echo "Syncing default resource types from resource-types-contrib..."
        copy_manifests
        prune_stale
        echo "Done. Review and commit the updated files."
        return 0
    fi

    # Recipe packs are pinned but never vendored, so this mode does not copy.
    if [ "${mode}" = update-recipe-packs ]; then
        if has_pins "${RECIPE_PACKS_PINS}"; then
            apply_pins "${RECIPE_PACKS_SECTION}" "${RECIPE_PACKS_PINS}" RECIPE_PACKS_PINS
        else
            update_section "${RECIPE_PACKS_SECTION}" "${RECIPE_PACKS_REF}" "${RECIPE_PACKS_NAME}"
        fi
        promote_edge_pins "${RECIPE_PACKS_SECTION}"
        validate_section_pins "${RECIPE_PACKS_SECTION}"
        echo "Done. Review and commit the updated ${DEFAULTS_YAML}."
        return 0
    fi

    if [ "${mode}" = update ]; then
        if has_pins "${RESOURCE_TYPES_PINS}"; then
            apply_pins "${RESOURCE_TYPES_SECTION}" "${RESOURCE_TYPES_PINS}" RESOURCE_TYPES_PINS
        else
            update_section "${RESOURCE_TYPES_SECTION}" "${RESOURCE_TYPES_REF}" "${RESOURCE_TYPES_NAMESPACE}"
        fi
        promote_edge_pins "${RESOURCE_TYPES_SECTION}"
    fi

    validate_section_pins "${RESOURCE_TYPES_SECTION}"
    validate_section_pins "${RECIPE_PACKS_SECTION}"
    echo "Syncing default resource types from resource-types-contrib..."
    copy_manifests
    prune_stale
    echo "Done. Review and commit the updated files."
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
