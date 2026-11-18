#!/bin/bash
# Copyright 2026 The Radius Authors. Licensed under the Apache License, Version 2.0.
# Compare committed final inputs offline. Requires Node.js, Git, and yq v4.

set -euo pipefail

exec node "$(dirname "${BASH_SOURCE[0]}")/verify-aws-final-inputs.mjs" "$@"
