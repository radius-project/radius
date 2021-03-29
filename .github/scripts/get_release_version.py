# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

# This script parses release version from Git tag and set the parsed version to
# environment variable, REL_VERSION.

import os
import re
import sys

gitRef = os.getenv("GITHUB_REF")
tagRefRegex = r"refs/tags/v(.*)"
pullRefRegex = r"refs/pull/(.*)/(.*)"

with open(os.getenv("GITHUB_ENV"), "a") as githubEnv:
    if gitRef is None:
        print("This is not running in github, GITHUB_REF is null. Assuming a local build...")
        version = "REL_VERSION=edge"
        print("Setting: {}".format(version))
        githubEnv.write(version + "\n")
        sys.exit(0)

    match = re.search(pullRefRegex, gitRef)
    if match is not None:
        print("This is pull request {}...".format(match.group(1)))
        version = "REL_VERSION=pr-{}".format(match.group(1))
        print("Setting: {}".format(version))
        githubEnv.write(version + "\n")
        sys.exit(0)

    match = re.search(tagRefRegex, gitRef)
    if match is not None:
        print("This is tagged as {}...".format(match.group(1)))
        version = "REL_VERSION={}".format(match.group(1))
        print("Setting: {}".format(version))
        githubEnv.write(version + "\n")

        if version.find("-rc") > 0:
            print("Release Candidate build from {}...".format(version))
        else:
            print("Release build from {}...".format(version))
            githubEnv.write("LATEST_RELEASE=true\n")
        sys.exit(0)

    githubEnv.write("\n")
    print("This is daily build from {}...".format(gitRef))
    version = "REL_VERSION=edge"
    print("Setting: {}".format(version))
    githubEnv.write(version + "\n")
    sys.exit(0)
