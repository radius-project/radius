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

VERSION=$1 # v0.21.0 or v0.21.0-rc1
VERSION_NUMBER=$2 # 0.21.0 or 0.21.0-rc1
CHANNEL=$3 # 0.21
CHANNEL_WITH_V="v${CHANNEL}" # v0.21

if [[ -d "samples" ]]; then
  echo "samples directory exists"
else
  echo "Error: samples directory does not exist. Please clone project-radius/samples repository."
  exit 1
fi

git checkout edge
git pull origin edge
git checkout -b "${CHANNEL_WITH_V}"
git pull origin "${CHANNEL_WITH_V}"
git push origin "${CHANNEL_WITH_V}"
