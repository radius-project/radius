#!/bin/bash

# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

# ============================================================================
# Purge old pr-* tagged GHCR container images under the org's dev/* packages.
#
# Replaces the snok/container-retention-policy action. GHCR's list-versions API
# is eventually consistent and keeps returning versions that were already
# deleted (with their old tags) for days, so a naive delete loop logs thousands
# of harmless "404 Not Found" errors every run. There is no API to force GHCR to
# refresh its listing, so this script treats 404/410 as the expected "already
# gone" state (silent) and persists those version ids in a "ghost" cache so
# subsequent runs skip them entirely.
#
# The ghost cache is self-healing: each run persists only the already-gone ids
# it actually observed this run (cached ids GHCR still lists, plus this run's
# fresh 404s and successful deletes). An id that GHCR finally stops listing is
# simply not re-observed, so it drops out of the cache on the next run. This
# keeps the cache bounded to the current staleness backlog without needing
# age-based expiry; GHOST_CACHE_MAX is only a hard backstop.
#
# Only throwaway dev/* pr-* CI images are ever touched, and only when every one
# of a version's tags matches the pr-* pattern, so latest/longr* shared images
# are never deleted. Deleting a tagged parent index cascade-removes its
# multi-arch children in GHCR, and orphaning a week-old test child is harmless,
# so no separate multi-arch protection is needed.
#
# Auth: GITHUB_TOKEN must be a classic PAT with read:packages + delete:packages.
# Deps: gh, jq.
# ============================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME

# Defaults (override via flags or environment).
ORG="${ORG:-radius-project}"
IMAGE_PREFIX="${IMAGE_PREFIX:-dev/}"
TAG_REGEX="${TAG_REGEX:-^pr-}"
CUTOFF_DAYS="${CUTOFF_DAYS:-7}"
GHOST_CACHE_FILE="${GHOST_CACHE_FILE:-.ghcr-ghost-cache/ghosts.txt}"
DRY_RUN="${DRY_RUN:-false}"
# Space deletes to stay under the ~180 deletes/min secondary rate limit.
DELETE_PACING_MS="${DELETE_PACING_MS:-350}"
# Cap the ghost cache so it cannot grow without bound.
GHOST_CACHE_MAX="${GHOST_CACHE_MAX:-20000}"

# Counters.
DELETED=0
GHOST_CACHED_SKIPS=0
GHOST_NEW=0
REAL_FAILURES=0
LOADED_GHOSTS=0

# GHOST_IDS: already-gone ids loaded from the cache; looked up to skip this run.
# KEEP_IDS:  already-gone ids to persist for next run (self-healing set) - the
#            cached ids GHCR still lists plus this run's fresh 404s and deletes.
declare -A GHOST_IDS=()
declare -A KEEP_IDS=()
declare -a REAL_FAILURE_IDS=()

usage() {
    echo "Usage: ${SCRIPT_NAME} [OPTIONS]"
    echo "Options:"
    echo "  -o, --org NAME           Organization (default: radius-project)"
    echo "  -p, --prefix STR         Image name prefix to match (default: dev/)"
    echo "  -c, --cutoff-days N      Delete versions older than N days (default: 7)"
    echo "  -g, --ghost-cache FILE   Ghost id cache file"
    echo "  -d, --dry-run            List candidates without deleting"
    echo "  -h, --help               Show this help"
    exit 0
}

validate_requirements() {
    for tool in gh jq curl; do
        if ! command -v "${tool}" > /dev/null 2>&1; then
            echo "Error: required tool '${tool}' is not installed" >&2
            exit 1
        fi
    done
    TOKEN="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
    if [[ -z "${TOKEN}" ]]; then
        echo "Error: GH_TOKEN/GITHUB_TOKEN (classic PAT) is required" >&2
        exit 1
    fi
    # gh reads GH_TOKEN; keep it in sync with whatever the caller provided.
    export GH_TOKEN="${TOKEN}"
}

load_ghost_cache() {
    if [[ -f "${GHOST_CACHE_FILE}" ]]; then
        local id
        while IFS= read -r id; do
            [[ -n "${id}" ]] && GHOST_IDS["${id}"]=1
        done < "${GHOST_CACHE_FILE}"
    fi
    LOADED_GHOSTS="${#GHOST_IDS[@]}"
    echo "Loaded ${LOADED_GHOSTS} ghost version ids from ${GHOST_CACHE_FILE}"
}

# Print dev/* container package names, one per line.
list_packages() {
    local url names
    url="/orgs/${ORG}/packages?package_type=container&per_page=100"
    names="$(gh api --paginate "${url}" --jq '.[].name')"
    grep -E "^${IMAGE_PREFIX}" <<< "${names}" || true
}

# Print "<id> <tab> <encoded-package>" for each deletable version of a package.
# A version is deletable when it has at least one tag, every tag matches
# TAG_REGEX, and it is older than the cutoff.
list_deletable_versions() {
    local pkg="$1"
    local encoded cutoff_epoch url raw ids id
    encoded="${pkg//\//%2F}"
    cutoff_epoch="$(date -u -d "${CUTOFF_DAYS} days ago" +%s)"
    url="/orgs/${ORG}/packages/container/${encoded}/versions?per_page=100"

    raw="$(gh api --paginate "${url}")"
    ids="$(jq -r --arg re "${TAG_REGEX}" --argjson cutoff "${cutoff_epoch}" '
        .[]
        | {id, updated_at, tags: (.metadata.container.tags // [])}
        | select((.tags | length) > 0)
        | select([.tags[] | test($re)] | all)
        | select((.updated_at | fromdateiso8601) < $cutoff)
        | .id
    ' <<< "${raw}")"

    while IFS= read -r id; do
        [[ -n "${id}" ]] && printf '%s\t%s\n' "${id}" "${encoded}"
    done <<< "${ids}"
}

# Delete a single version. Returns 0 on delete/already-gone, 1 on real failure.
delete_version() {
    local id="$1" encoded="$2"
    local attempt=0 max_attempts=5 status wait

    while ((attempt < max_attempts)); do
        ((attempt += 1))
        status="$(curl -sS -o /dev/null -w '%{http_code}' \
            -X DELETE \
            -H "Authorization: Bearer ${TOKEN}" \
            -H "Accept: application/vnd.github+json" \
            -H "X-GitHub-Api-Version: 2022-11-28" \
            "https://api.github.com/orgs/${ORG}/packages/container/${encoded}/versions/${id}")"

        case "${status}" in
            204)
                # We just deleted it; keep it so next run skips it instead of
                # re-issuing a delete that GHCR would answer with a 404.
                ((DELETED += 1))
                KEEP_IDS["${id}"]=1
                return 0
                ;;
            404 | 410)
                # Already gone: skip it silently now and on future runs.
                GHOST_IDS["${id}"]=1
                KEEP_IDS["${id}"]=1
                ((GHOST_NEW += 1))
                return 0
                ;;
            403 | 429)
                # Secondary rate limit: back off and retry.
                wait=$((attempt * attempt * 5))
                echo "Rate limited on ${id} (status ${status}); backing off ${wait}s" >&2
                sleep "${wait}"
                ;;
            *)
                echo "Real failure deleting ${id} (status ${status})" >&2
                REAL_FAILURE_IDS+=("${id}")
                ((REAL_FAILURES += 1))
                return 1
                ;;
        esac
    done

    echo "Giving up on ${id} after ${max_attempts} attempts" >&2
    REAL_FAILURE_IDS+=("${id}")
    ((REAL_FAILURES += 1))
    return 1
}

write_ghost_cache() {
    local dir sorted
    dir="$(dirname "${GHOST_CACHE_FILE}")"
    mkdir -p "${dir}"
    # Persist only the ids observed this run (self-healing eviction). sort -n so
    # the GHOST_CACHE_MAX backstop keeps the newest (highest) ids if ever hit.
    sorted="$(printf '%s\n' "${!KEEP_IDS[@]}" | grep -E '^[0-9]+$' | sort -n -u)"
    if [[ -n "${sorted}" ]]; then
        tail -n "${GHOST_CACHE_MAX}" <<< "${sorted}" > "${GHOST_CACHE_FILE}"
    else
        : > "${GHOST_CACHE_FILE}"
    fi
    echo "Wrote $(wc -l < "${GHOST_CACHE_FILE}") ghost ids to ${GHOST_CACHE_FILE}"
}

main() {
    validate_requirements
    load_ghost_cache

    echo "=============================================================="
    echo "Purging ${IMAGE_PREFIX}* images older than ${CUTOFF_DAYS}d in ${ORG}"
    echo "Tag filter: ${TAG_REGEX}  Dry run: ${DRY_RUN}"
    echo "=============================================================="

    local packages=()
    mapfile -t packages < <(list_packages)
    echo "Found ${#packages[@]} ${IMAGE_PREFIX}* packages"

    local candidates=0
    local pkg id encoded
    for pkg in "${packages[@]}"; do
        [[ -z "${pkg}" ]] && continue
        while IFS=$'\t' read -r id encoded; do
            [[ -z "${id}" ]] && continue
            ((candidates += 1))

            if [[ -n "${GHOST_IDS[${id}]:-}" ]]; then
                # GHCR still lists this known-gone id; keep skipping it next run.
                ((GHOST_CACHED_SKIPS += 1))
                KEEP_IDS["${id}"]=1
                continue
            fi

            if [[ "${DRY_RUN}" == "true" ]]; then
                echo "[dry-run] would delete ${pkg} version ${id}"
                continue
            fi

            delete_version "${id}" "${encoded}" || true
            sleep "$(awk "BEGIN{print ${DELETE_PACING_MS}/1000}")"
        done < <(list_deletable_versions "${pkg}")
    done

    if [[ "${DRY_RUN}" != "true" ]]; then
        write_ghost_cache
    fi

    echo "=============================================================="
    echo "Summary"
    echo "  packages scanned : ${#packages[@]}"
    echo "  candidates       : ${candidates}"
    echo "  deleted          : ${DELETED}"
    echo "  already-gone     : $((GHOST_CACHED_SKIPS + GHOST_NEW)) (cached ${GHOST_CACHED_SKIPS}, new-404 ${GHOST_NEW})"
    echo "  cache evicted    : $((LOADED_GHOSTS - GHOST_CACHED_SKIPS)) (loaded ${LOADED_GHOSTS}, no longer listed)"
    echo "  real failures    : ${REAL_FAILURES}"
    echo "=============================================================="

    if ((REAL_FAILURES > 0)); then
        echo "Failed version ids: ${REAL_FAILURE_IDS[*]}" >&2
        exit 1
    fi
}

while [[ $# -gt 0 ]]; do
    case $1 in
        -o | --org)
            ORG="$2"
            shift 2
            ;;
        -p | --prefix)
            IMAGE_PREFIX="$2"
            shift 2
            ;;
        -c | --cutoff-days)
            CUTOFF_DAYS="$2"
            shift 2
            ;;
        -g | --ghost-cache)
            GHOST_CACHE_FILE="$2"
            shift 2
            ;;
        -d | --dry-run)
            DRY_RUN="true"
            shift
            ;;
        -h | --help)
            usage
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

main "$@"
