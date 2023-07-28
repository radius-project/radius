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

FILENAME=$1

if [ -z "$FILENAME" ]; then
  echo "No filename provided"
  exit 1
fi

touch $FILENAME
echo "========== kubectl get pods -A ==========\n" >> $FILENAME
kubectl get pods -A >> $FILENAME
echo "=========================================\n\n" >> $FILENAME
echo "========== kubectl describe pods -A ==========\n" >> $FILENAME
kubectl describe pods -A >> $FILENAME
echo "==============================================" >> $FILENAME
