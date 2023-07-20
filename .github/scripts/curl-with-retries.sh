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

# See https://everything.curl.dev/usingcurl/downloads/retry
#
# Unfortunately we can't use --retry-all-errors as the agent does not support it.

# Number of retries
MAX_RETRIES=5

# Retry delay in seconds
RETRY_DELAY=5

# Retry loop
RETRY_COUNT=0
while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    # Download the file with retries and resume capability
    curl --retry $MAX_RETRIES --retry-delay $RETRY_DELAY --continue-at - $@

    # Check if the download was successful
    if [ $? -eq 0 ]; then
        echo "File downloaded successfully"
        break
    fi

    # Download failed, increase the retry count
    RETRY_COUNT=$((RETRY_COUNT + 1))

    # Check if there are more retries left
    if [ $RETRY_COUNT -lt $MAX_RETRIES ]; then
        # Retry after delay
        echo "Retrying in $RETRY_DELAY seconds..."
        sleep $RETRY_DELAY
    fi
done

# Check if the maximum number of retries exceeded
if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "Maximum number of retries exceeded"
fi
