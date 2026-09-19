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

# This script validates that the provided version is valid SemVer.

import re
import sys

# Adapted from the suggested SemVer 2.0.0 regular expression:
# https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
# All groups are non-capturing because only match success is used.
SEMVER_PATTERN = re.compile(
    r"(?:0|[1-9][0-9]*)\."
    r"(?:0|[1-9][0-9]*)\."
    r"(?:0|[1-9][0-9]*)"
    r"(?:-(?:(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)"
    r"(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?"
    r"(?:\+(?:[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
)


def is_valid_semver(version: str) -> bool:
    return SEMVER_PATTERN.fullmatch(version) is not None


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: validate_semver.py <version>")
        return 1

    version = sys.argv[1]
    if not is_valid_semver(version):
        print("Provided version is not valid semver")
        return 1

    print("Provided version is valid semver")
    return 0


if __name__ == "__main__":
    sys.exit(main())
