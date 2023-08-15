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

set -xe

# Repository to create tags and branches for
# (e.g. radius)
REPOSITORY=$1

# Comma-separated list of tag versions 
# (e.g. v0.2.0,v0.1.0)
VERSIONS=$2

if [[ -z "$VERSIONS" ]]; then
  echo "Error: VERSIONS is not set."
  exit 1
fi

# Create the tags and branches for each version
for VERSION in $(echo $VERSIONS | sed "s/,/ /g")
do
  sh .github/scripts/release-create-tag-and-branch.sh $REPOSITORY $VERSION
done
