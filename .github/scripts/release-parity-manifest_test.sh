#!/bin/bash

# ------------------------------------------------------------
# Copyright 2026 The Radius Authors.
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

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
TEST_ROOT="$(mktemp -d)"
readonly TEST_ROOT
trap 'rm -rf "${TEST_ROOT}"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

readonly FAKE_BIN="${TEST_ROOT}/bin"
readonly FIXTURES="${TEST_ROOT}/fixtures"
readonly ASSETS="${FIXTURES}/assets"
readonly OUTPUT="${TEST_ROOT}/manifest.json"
readonly FIRST_OUTPUT="${TEST_ROOT}/manifest-first.json"
readonly TARGETS="${FIXTURES}/targets.json"
readonly SOURCE_COMMIT="0123456789abcdef0123456789abcdef01234567"
export FIXTURES SOURCE_COMMIT
OMIT_ARM="false"
MISSING_DOWNSTREAM="false"
export OMIT_ARM MISSING_DOWNSTREAM

mkdir -p "${FAKE_BIN}" "${ASSETS}"

cat >"${TARGETS}" <<'EOF'
{
  "repository": "radius-project/radius",
  "cliAssets": [
    {"name": "rad_linux_amd64", "os": "linux", "arch": "amd64"}
  ],
  "images": [
    {
      "category": "production",
      "name": "ucpd",
      "radiusBuild": true,
      "requiredPlatforms": [
        "linux/amd64",
        "linux/arm/v7",
        "linux/arm64"
      ]
    }
  ],
  "imageRegistry": "ghcr.io/radius-project",
  "helm": {
    "expectedImages": ["ucpd"],
    "ociReference": "oci://ghcr.io/radius-project/helm-chart/radius",
    "orasReference": "ghcr.io/radius-project/helm-chart/radius"
  },
  "siblingRepositories": ["radius-project/recipes"],
  "ociArtifacts": [
    {
      "artifactType": "application/vnd.ms.bicep.provider.artifact",
      "name": "radius-bicep-types",
      "repository": "biceptypes.azurecr.io/radius"
    }
  ]
}
EOF

cat >"${ASSETS}/rad_linux_amd64" <<EOF
#!/bin/bash
printf '%s\n' '{"release":"0.60.0","version":"v0.60.0","bicep":"0.42.1","commit":"${SOURCE_COMMIT}"}'
EOF
chmod +x "${ASSETS}/rad_linux_amd64"
binary_sha="$(sha256sum "${ASSETS}/rad_linux_amd64" | cut -d ' ' -f 1)"
printf '%s *%s\n' "${binary_sha}" "rad_linux_amd64" \
    >"${ASSETS}/rad_linux_amd64.sha256"

cat >"${FIXTURES}/release-note.md" <<'EOF'
# Radius v0.60.0
EOF

cat >"${FIXTURES}/image.json" <<EOF
{
  "name": "ghcr.io/radius-project/ucpd:0.60",
  "manifest": {
    "mediaType": "application/vnd.oci.image.index.v1+json",
    "digest": "sha256:index",
    "size": 100,
    "manifests": [
      {
        "mediaType": "application/vnd.oci.image.manifest.v1+json",
        "digest": "sha256:amd64",
        "size": 10,
        "platform": {"architecture": "amd64", "os": "linux"}
      },
      {
        "mediaType": "application/vnd.oci.image.manifest.v1+json",
        "digest": "sha256:armv7",
        "size": 10,
        "platform": {
          "architecture": "arm",
          "os": "linux",
          "variant": "v7"
        }
      },
      {
        "mediaType": "application/vnd.oci.image.manifest.v1+json",
        "digest": "sha256:arm64",
        "size": 10,
        "platform": {"architecture": "arm64", "os": "linux"}
      }
    ]
  },
  "image": {
    "linux/amd64": {
      "architecture": "amd64",
      "os": "linux",
      "config": {
        "User": "65532:65532",
        "Entrypoint": ["/ucpd"],
        "WorkingDir": "/",
        "Labels": {
          "org.opencontainers.image.description": "ucpd",
          "org.opencontainers.image.revision": "${SOURCE_COMMIT}",
          "org.opencontainers.image.source": "https://github.com/radius-project/radius",
          "org.opencontainers.image.version": "0.60.0"
        }
      }
    },
    "linux/arm/v7": {
      "architecture": "arm",
      "os": "linux",
      "variant": "v7",
      "config": {
        "User": "65532:65532",
        "Entrypoint": ["/ucpd"],
        "WorkingDir": "/",
        "Labels": {
          "org.opencontainers.image.description": "ucpd",
          "org.opencontainers.image.revision": "${SOURCE_COMMIT}",
          "org.opencontainers.image.source": "https://github.com/radius-project/radius",
          "org.opencontainers.image.version": "0.60.0"
        }
      }
    },
    "linux/arm64": {
      "architecture": "arm64",
      "os": "linux",
      "config": {
        "User": "65532:65532",
        "Entrypoint": ["/ucpd"],
        "WorkingDir": "/",
        "Labels": {
          "org.opencontainers.image.description": "ucpd",
          "org.opencontainers.image.revision": "${SOURCE_COMMIT}",
          "org.opencontainers.image.source": "https://github.com/radius-project/radius",
          "org.opencontainers.image.version": "0.60.0"
        }
      }
    }
  }
}
EOF

cat >"${FIXTURES}/release.json" <<EOF
{
  "name": "Radius v0.60.0",
  "body": "# Radius v0.60.0\n",
  "draft": false,
  "prerelease": false,
  "published_at": "2026-08-19T00:47:21Z",
  "html_url": "https://github.com/radius-project/radius/releases/tag/v0.60.0",
  "assets": [
    {
      "name": "rad_linux_amd64",
      "digest": "sha256:${binary_sha}"
    },
    {
      "name": "rad_linux_amd64.sha256",
      "digest": "sha256:$(sha256sum "${ASSETS}/rad_linux_amd64.sha256" | cut -d ' ' -f 1)"
    }
  ]
}
EOF

cat >"${FAKE_BIN}/gh" <<'EOF'
#!/bin/bash
set -euo pipefail

if [[ "$1" == "api" ]]; then
    endpoint="${*: -1}"
    case "${endpoint}" in
        */releases/tags/v0.60.0)
            cat "${FIXTURES}/release.json"
            ;;
        */git/ref/tags/v0.60.0)
            printf '{"object":{"type":"commit","sha":"%s"}}\n' \
                "${SOURCE_COMMIT}"
            ;;
        */contents/docs/release-notes/v0.60.0.md*)
            cat "${FIXTURES}/release-note.md"
            ;;
        *)
            echo "unexpected gh api endpoint: ${endpoint}" >&2
            exit 1
            ;;
    esac
elif [[ "$1 $2" == "release download" ]]; then
    output_dir=""
    while [[ $# -gt 0 ]]; do
        if [[ "$1" == "--dir" ]]; then
            output_dir="$2"
            break
        fi
        shift
    done
    cp "${FIXTURES}/assets/"* "${output_dir}/"
else
    echo "unexpected gh invocation: $*" >&2
    exit 1
fi
EOF
chmod +x "${FAKE_BIN}/gh"

cat >"${FAKE_BIN}/go" <<EOF
#!/bin/bash
set -euo pipefail
cat <<'JSON'
{
  "GoVersion": "go1.26.5",
  "Path": "github.com/radius-project/radius/cmd/rad",
  "Main": {
    "Path": "github.com/radius-project/radius",
    "Version": "v0.60.0",
    "Sum": ""
  },
  "Settings": [
    {"Key":"GOOS","Value":"linux"},
    {"Key":"GOARCH","Value":"amd64"},
    {"Key":"CGO_ENABLED","Value":"0"},
    {"Key":"vcs.revision","Value":"${SOURCE_COMMIT}"},
    {"Key":"-ldflags","Value":"-s -w -X github.com/radius-project/radius/pkg/version.channel=0.60 -X github.com/radius-project/radius/pkg/version.release=0.60.0 -X github.com/radius-project/radius/pkg/version.commit=${SOURCE_COMMIT} -X github.com/radius-project/radius/pkg/version.version=v0.60.0 -X github.com/radius-project/radius/pkg/version.chartVersion=0.60.0 -X github.com/radius-project/radius/pkg/recipes/terraform.terraformVersion=1.15.8"}
  ]
}
JSON
EOF
chmod +x "${FAKE_BIN}/go"

cat >"${FAKE_BIN}/docker" <<'EOF'
#!/bin/bash
set -euo pipefail

if [[ "$*" != "buildx imagetools inspect --format {{json .}} "* ]]; then
  echo "unexpected docker invocation: $*" >&2
  exit 1
fi

if [[ "${OMIT_ARM}" == "true" ]]; then
  jq '
    .manifest.manifests |= map(
      select(.platform.architecture != "arm")
    )
    | del(.image["linux/arm/v7"])
  ' "${FIXTURES}/image.json"
else
  cat "${FIXTURES}/image.json"
fi
EOF
chmod +x "${FAKE_BIN}/docker"

cat >"${FAKE_BIN}/helm" <<'EOF'
#!/bin/bash
set -euo pipefail

if [[ "$1 $2" == "show chart" ]]; then
  cat <<'YAML'
apiVersion: v2
name: radius
version: 0.60.0
appVersion: 0.60.0
description: Radius test chart
YAML
elif [[ "$1" == "template" ]]; then
  cat <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ucpd
spec:
  template:
  spec:
    containers:
    - name: ucpd
      image: ghcr.io/radius-project/ucpd:0.60
YAML
else
  echo "unexpected helm invocation: $*" >&2
  exit 1
fi
EOF
chmod +x "${FAKE_BIN}/helm"

cat >"${FAKE_BIN}/oras" <<'EOF'
#!/bin/bash
set -euo pipefail

reference="${*: -1}"
if [[ "${MISSING_DOWNSTREAM}" == "true" && \
  "${reference}" == biceptypes.azurecr.io/* ]]; then
  echo "artifact not found: ${reference}" >&2
  exit 1
fi

if [[ "$*" == *"--descriptor"* ]]; then
  printf '%s\n' '{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:descriptor","size":100}'
elif [[ "${reference}" == ghcr.io/radius-project/helm-chart/radius:* ]]; then
  printf '%s\n' '{"schemaVersion":2,"config":{"mediaType":"application/vnd.cncf.helm.config.v1+json","digest":"sha256:config","size":10},"layers":[{"mediaType":"application/vnd.cncf.helm.chart.content.v1.tar+gzip","digest":"sha256:chart","size":20}]}'
elif [[ "${reference}" == biceptypes.azurecr.io/radius:* ]]; then
  printf '%s\n' '{"schemaVersion":2,"artifactType":"application/vnd.ms.bicep.provider.artifact","config":{"mediaType":"application/vnd.ms.bicep.provider.config.v1+json","digest":"sha256:config","size":2},"layers":[{"mediaType":"application/vnd.ms.bicep.provider.layer.v1.tar+gzip","digest":"sha256:types","size":20}]}'
else
  echo "unexpected oras reference: ${reference}" >&2
  exit 1
fi
EOF
chmod +x "${FAKE_BIN}/oras"

cat >"${FAKE_BIN}/yq" <<'EOF'
#!/bin/bash
set -euo pipefail

if [[ "$1" == "eval" ]]; then
    cat <<'JSON'
{
  "apiVersion": "v2",
  "name": "radius",
  "version": "0.60.0",
  "appVersion": "0.60.0",
  "description": "Radius test chart"
}
JSON
elif [[ "$1" == "eval-all" ]]; then
    cat <<'JSON'
[
  {
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    "metadata": {"name": "ucpd"},
    "spec": {
      "template": {
        "spec": {
          "containers": [
            {
              "name": "ucpd",
              "image": "ghcr.io/radius-project/ucpd:0.60"
            }
          ]
        }
      }
    }
  }
]
JSON
else
    echo "unexpected yq invocation: $*" >&2
    exit 1
fi
EOF
chmod +x "${FAKE_BIN}/yq"

run_collector() {
    PATH="${FAKE_BIN}:${PATH}" \
        RELEASE_PARITY_TARGETS="${TARGETS}" \
        RELEASE_PARITY_OBSERVED_AT="2026-08-19T01:00:00Z" \
        bash "${SCRIPT_DIR}/release-parity-manifest.sh" \
        --version 0.60.0 \
        --output "${OUTPUT}" >/dev/null
}

run_collector
cp "${OUTPUT}" "${FIRST_OUTPUT}"
run_collector
cmp -s "${FIRST_OUTPUT}" "${OUTPUT}" ||
  fail "collector output was not deterministic"

jq -e '
    .schemaVersion == 1
    and .release.sourceCommit == $commit
    and .release.notes.source.type == "repository-file"
    and .release.notes.matchesSource == true
    and (.cli.assets | length) == 1
    and .cli.assets[0].checksum.valid == true
    and .cli.assets[0].build.goVersion == "go1.26.5"
    and .cli.assets[0].build.linkerMetadata.channel == "0.60"
    and .cli.runtimeVersion.release == "0.60.0"
    and (.images | length) == 1
    and (.images[0].platforms | length) == 3
    and .images[0].platforms[2].platform == "linux/arm64"
    and .helm.metadata.version == "0.60.0"
    and .helm.renderedImages == ["ghcr.io/radius-project/ucpd:0.60"]
    and .downstream.repositories[0].repository == "radius-project/recipes"
    and .downstream.ociArtifacts[0].name == "radius-bicep-types"
' --arg commit "${SOURCE_COMMIT}" "${OUTPUT}" >/dev/null ||
    fail "generated manifest did not match the expected contract"

jq '.prerelease = true | .body = "<!-- Release notes generated using configuration -->\n"' \
  "${FIXTURES}/release.json" >"${FIXTURES}/release-rc.json"
mv "${FIXTURES}/release-rc.json" "${FIXTURES}/release.json"
run_collector
jq -e '
  .release.prerelease == true
  and .release.notes.source.type == "github-generated"
  and .release.notes.matchesSource == false
' "${OUTPUT}" >/dev/null || fail "collector did not identify generated RC notes"
jq '.prerelease = false | .body = "# Radius v0.60.0\n"' \
  "${FIXTURES}/release.json" >"${FIXTURES}/release-final.json"
mv "${FIXTURES}/release-final.json" "${FIXTURES}/release.json"

printf '%064d *rad_linux_amd64\n' 0 \
    >"${ASSETS}/rad_linux_amd64.sha256"
if run_collector 2>/dev/null; then
    fail "collector accepted an invalid checksum"
fi

printf '%s *%s\n' "${binary_sha}" "rad_linux_amd64" \
  >"${ASSETS}/rad_linux_amd64.sha256"
OMIT_ARM="true"
if run_collector 2>/dev/null; then
  fail "collector accepted a missing runtime platform"
fi
OMIT_ARM="false"

MISSING_DOWNSTREAM="true"
if run_collector 2>/dev/null; then
  fail "collector accepted a missing downstream artifact"
fi
MISSING_DOWNSTREAM="false"

for baseline in "${SCRIPT_DIR}/../release-parity/baselines/"*.json; do
  jq -e --slurpfile targets "${SCRIPT_DIR}/../release-parity/targets.json" '
    . as $manifest
    | ($targets[0].images
      | map({key: .name, value: (.requiredPlatforms | sort)})
      | from_entries) as $expected_platforms
    | .schemaVersion == 1
    and .release.tag == ("v" + .release.version)
    and .release.draft == false
    and ([.cli.assets[].name] | sort)
      == ([$targets[0].cliAssets[].name] | sort)
    and all(.cli.assets[];
      .checksum.valid == true
      and .build.linkerMetadata.release == $manifest.release.version
      and .build.linkerMetadata.commit
        == $manifest.release.sourceCommit
    )
    and ([.images[].name] | sort)
      == ([$targets[0].images[].name] | sort)
    and all(.images[];
      ([.platforms[].platform] | sort)
        == $expected_platforms[.name]
    )
    and .helm.metadata.name == "radius"
    and .helm.metadata.version == .release.version
    and .helm.metadata.appVersion == .release.version
    and ([.downstream.repositories[].repository] | sort)
      == ($targets[0].siblingRepositories | sort)
    and ([.downstream.ociArtifacts[].name] | sort)
      == ([$targets[0].ociArtifacts[].name] | sort)
  ' "${baseline}" >/dev/null ||
    fail "committed baseline failed validation: ${baseline}"
done

echo "release parity manifest tests passed"
