#!/bin/bash
find . -type f -name "*.bicep" -exec sed -i '' -e "s/magpiego:latest/magpiego:${REL_VERSION}/g" {} +