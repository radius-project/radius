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

set -e

FLUX_VERSION=$1

if [ -z "$FLUX_VERSION" ]; then
  echo "FLUX_VERSION is not set. Exiting..."
  exit 1
fi

for i in 1 2 3; do
  curl -s https://fluxcd.io/install.sh | FLUX_VERSION=$FLUX_VERSION sudo bash && \
  flux install --namespace=flux-system --version=v"$FLUX_VERSION" --components=source-controller --network-policy=false && \
  kubectl wait --for=condition=available deployment -l app.kubernetes.io/component=source-controller -n flux-system --timeout=120s && break
  echo "Attempt $i failed, retrying in 10 seconds..."
  sleep 10
done
