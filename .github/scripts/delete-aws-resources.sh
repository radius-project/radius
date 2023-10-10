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

# Comma-separated list of AWS resource types
RESOURCE_TYPES=$1

# Label on AWS resources
LABEL='RadiusCreationTimestamp'

# File to store the list of deleted resources
DELETED_RESOURCES_FILE='deleted-resources.txt'

# Number of retries
MAX_RETRIES=5

# Retry delay in seconds
RETRY_DELAY=300 # 5 minutes

# Maximum age of resources in seconds
MAX_AGE=21600

# Current time in seconds
CURRENT_TIME=$(date +%s)

function delete_old_aws_resources() {
  # Empty the file
  truncate -s 0 $DELETED_RESOURCES_FILE

  for resource_type in ${RESOURCE_TYPES//,/ }
  do
    aws cloudcontrol list-resources --type-name "$resource_type" --query "ResourceDescriptions[].Identifier" --output text | tr '\t' '\n' | while read identifier
    do
      aws cloudcontrol get-resource --type-name "$resource_type" --identifier "$identifier" --query "ResourceDescription.Properties" --output text | while read resource
      do
        resource_tags=$(jq -c -r .Tags <<< "$resource")
        for tag in $(jq -c -r '.[]' <<< "$resource_tags")
        do
          key=$(jq -r '.Key' <<< "$tag")
          value=$(jq -r '.Value' <<< "$tag")
          if [[ "$key" == "$LABEL" && $((CURRENT_TIME - value)) -gt $MAX_AGE]]
          then
            echo "Deleting resource of type: $resource_type with identifier: $identifier"
            echo "$identifier\n" >> $DELETED_RESOURCES_FILE
            aws cloudcontrol delete-resource --type-name "$resource_type" --identifier "$identifier"
          fi
        done
      done
    done
  done

  if [ -s $DELETED_RESOURCES_FILE ]; then
    return 1
  else
    return 0
  fi
}

RETRY_COUNT=0
while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    # Trigger the function to delete the resources
    delete_old_aws_resources

    # If the function returned 0, then no resources needed to be deleted
    # on this run. This means that all resources have been deleted.
    if [ $? -eq 0 ]; then
        echo "All resources deleted successfully"
        break
    fi

    # Still have resources to delete, increase the retry count
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
