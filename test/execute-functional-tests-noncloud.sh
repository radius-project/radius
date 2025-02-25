#!/usr/bin/env bash

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

# Make sure that you have default environment in the default group.
# The way to check that is to run the following commands:
# `rad group switch default`
# `rad env list` and check if there is an environment named `default`.

# To run functional tests, you have to be in `kind-radius` group and
# `kind-radius` environment within the `default` group. The reason for this
# is that we have used `kind-radius` in some tests in a hardcoded way.
# You can switch to the group using `rad group switch kind-radius`
# and then switch to the environment using `rad env switch kind-radius`.

rad group create default
rad env create kind-radius
rad group create kind-radius

# Check if TF_RECIPE_MODULE_SERVER_URL environment variable is set
if [ -z "$TF_RECIPE_MODULE_SERVER_URL" ]; then
  echo "Error: TF_RECIPE_MODULE_SERVER_URL environment variable is not set."
  exit 1
fi

# Check if DOCKER_REGISTRY environment variable is set
if [ -z "$DOCKER_REGISTRY" ]; then
  echo "Error: DOCKER_REGISTRY environment variable is not set."
  exit 1
fi

# Check if BICEP_RECIPE_REGISTRY environment variable is set
if [ -z "$BICEP_RECIPE_REGISTRY" ]; then
  echo "Error: BICEP_RECIPE_REGISTRY environment variable is not set."
  exit 1
fi

# Check if RADIUS_SAMPLES_REPO_ROOT environment variable is set
if [ -z "$RADIUS_SAMPLES_REPO_ROOT" ]; then
  echo "Error: RADIUS_SAMPLES_REPO_ROOT environment variable is not set."
  exit 1
fi

# You can run them one by one or all at once.
# The way to run them all at once is to run the following command:
# `make test-functional-all-noncloud`

# Make sure that you are in the root directory of the repository.
cd ../

echo "Running Radius non-cloud functional tests."

make test-functional-cli-noncloud
# make test-functional-corerp-noncloud
make test-functional-daprrp-noncloud
make test-functional-datastoresrp-noncloud
make test-functional-kubernetes-noncloud
# make test-functional-msgrp-noncloud
make test-functional-samples-noncloud
make test-functional-ucp-noncloud
