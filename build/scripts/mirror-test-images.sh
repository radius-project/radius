#!/usr/bin/env bash
# Copy upstream multi-arch container images into the
# ghcr.io/radius-project/mirror/* namespace, preserving the full OCI index
# (i.e. all platforms) so the same tag works on amd64 (CI) and arm64
# (Apple Silicon dev machines / k3d). Uses `docker buildx imagetools create`,
# which performs a server-side blob mount when source and destination are on
# the same registry, and otherwise a streaming copy.
#
# Requirements:
#   - docker (with buildx, included in Docker Desktop)
#   - You must be logged in to ghcr.io with a token that has write:packages.
#     Easiest: `echo $GH_PACKAGES_TOKEN | docker login ghcr.io -u <user> --password-stdin`
#     The token used by `gh auth login` does NOT include write:packages by default;
#     create a Classic PAT with the `write:packages` scope.
#
# Usage:
#   build/scripts/mirror-test-images.sh           # mirror the default list below
#   build/scripts/mirror-test-images.sh --dry-run # show what would be copied
#   build/scripts/mirror-test-images.sh src=dst   # mirror a single mapping
#
# Each mapping is SRC=DST_TAG where:
#   SRC     = fully-qualified upstream reference (e.g. docker.io/library/rabbitmq:3.12-management-alpine)
#   DST_TAG = destination repo+tag under ghcr.io/radius-project/mirror/
#             (e.g. rabbitmq:3.12-management-alpine)

set -euo pipefail

DEST_PREFIX="ghcr.io/radius-project/mirror"

# Default mappings. Every source MUST be multi-arch (at least linux/amd64 +
# linux/arm64) for the destination to be usable from both CI and arm64 dev
# machines. Verify with `docker buildx imagetools inspect <src>` before adding.
DEFAULT_MAPPINGS=(
  # These mirror the exact same tags already referenced by tests / recipes.
  # The previous mirror entries were pushed as single-arch (linux/amd64 only)
  # blobs, which breaks tests on arm64 dev machines (e.g. rabbitmq 3.10 BEAM
  # crashes under QEMU). Re-mirroring with buildx imagetools create copies the
  # full upstream OCI index so all platforms work.
  "docker.io/library/rabbitmq:3.10=rabbitmq:3.10"
  "docker.io/library/redis:6.2=redis:6.2"
  "docker.io/library/mongo:4.2=mongo:4.2"
  "docker.io/library/postgres:latest=postgres:latest"
  "docker.io/library/debian:latest=debian:latest"
)

dry_run=0
mappings=()
for arg in "$@"; do
  case "$arg" in
    --dry-run) dry_run=1 ;;
    -h|--help) sed -n '1,30p' "$0"; exit 0 ;;
    *=*) mappings+=("$arg") ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

if [[ ${#mappings[@]} -eq 0 ]]; then
  mappings=("${DEFAULT_MAPPINGS[@]}")
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2; exit 1
fi
if ! docker buildx version >/dev/null 2>&1; then
  echo "docker buildx is required (included with Docker Desktop)" >&2; exit 1
fi

echo "Mirroring ${#mappings[@]} image(s) to ${DEST_PREFIX}/*"
for m in "${mappings[@]}"; do
  src="${m%%=*}"
  dst_suffix="${m#*=}"
  dst="${DEST_PREFIX}/${dst_suffix}"
  echo
  echo "==> ${src}"
  echo "    -> ${dst}"

  # Verify source is multi-arch before copying. The destination will inherit
  # whatever platforms the source has, so a single-arch source means we did
  # not solve the original problem.
  platforms="$(docker buildx imagetools inspect --raw "${src}" 2>/dev/null \
    | python3 -c '
import json,sys
try:
  d=json.load(sys.stdin)
except Exception:
  print(""); sys.exit(0)
m=d.get("manifests")
if not m:
  print(""); sys.exit(0)
print(",".join(sorted({x["platform"]["os"]+"/"+x["platform"]["architecture"]
  for x in m if "platform" in x and x["platform"].get("os")!="unknown"})))
')"
  if [[ -z "${platforms}" ]]; then
    echo "    !! source is single-arch (no manifest index); refusing to mirror" >&2
    exit 1
  fi
  if [[ "${platforms}" != *"linux/amd64"* || "${platforms}" != *"linux/arm64"* ]]; then
    echo "    !! source must include both linux/amd64 and linux/arm64; got: ${platforms}" >&2
    exit 1
  fi
  echo "    platforms: ${platforms}"

  if [[ "${dry_run}" -eq 1 ]]; then
    continue
  fi
  docker buildx imagetools create --tag "${dst}" "${src}"
done

echo
echo "Done."
