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

# This script parses release version from Git tag and set the parsed version to
# environment variable, REL_VERSION.

# We set the environment variable REL_CHANNEL based on the kind of build. This is used for
# versioning of our assets.
#
# REL_CHANNEL is:
# 'edge': for most builds
# 'edge': for PR builds
# '1.0.0-rc1' (the full version): for a tagged prerelease
# '1.0' (major.minor): for a tagged release

# LATEST_STABLE_CHANNEL is the first non-prerelease channel in versions.yaml.
# We set UPDATE_RELEASE for any full release and UPDATE_LATEST when that release
# belongs to LATEST_STABLE_CHANNEL.

# We set the environment variable CHART_VERSION based on the kind of build. This is used for
# versioning our helm chart ONLY.
#
# CHART_VERSION is:
#
# '0.42.42-dev' for most builds
# '0.42.42-pr-<pull request #>' for PR builds
# '1.0.0-rc1' (the full version): for a tagged prerelease
# '1.0.0' (major.minor.patch): for a tagged release
#
# note: we always install the helm chart using the tilde-range syntax to match our behavior
# based on channels. For example '--version ~0.9.0' will install the latest 0.9.X build
# which matches how we do other versioning/installation operations.

# REL_VERSION is used to stamp versions into binaries
# REL_CHANNEL is used to upload assets to different paths
# CHART_VERSION is used to set the version of the helm chart

# This way a 1.0 user can download 1.0.1, etc.

import os
import re
import sys
from pathlib import Path


def get_supported_releases(versions_file):
    in_supported = False
    channel_pattern = re.compile(
        r"^\s*-\s+channel:\s*(['\"]?)(?P<channel>\d+\.\d+)\1\s*(?:#.*)?$")
    version_pattern = re.compile(
        r"^\s+version:\s*(['\"]?)(?P<version>v[^'\"\s]+)\1\s*(?:#.*)?$")
    candidateChannel = None
    releases = []

    with open(versions_file, encoding="utf-8") as versions:
        for line in versions:
            if line.strip() == "supported:":
                in_supported = True
                continue

            if in_supported and line and not line[0].isspace():
                break

            if in_supported:
                channelMatch = channel_pattern.match(line)
                if channelMatch is not None:
                    candidateChannel = channelMatch.group("channel")
                    continue

                versionMatch = version_pattern.match(line)
                if versionMatch is not None and candidateChannel is not None:
                    releases.append((
                        candidateChannel,
                        versionMatch.group("version").removeprefix("v")))
                    candidateChannel = None

    if not releases:
        raise ValueError(
            "Could not find supported releases in {}".format(versions_file))
    return releases

gitRef = os.getenv("GITHUB_REF")
githubEnvPath = os.getenv("GITHUB_ENV")
if githubEnvPath is None:
    raise ValueError("GITHUB_ENV must be set")

# From https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
# Group 'version' returns the whole version
# other named groups return the components
tagRefRegex = r"^refs/tags/v(?P<version>0|(?P<major>0|[1-9]\d*)\.(?P<minor>0|[1-9]\d*)\.(?P<patch>0|[1-9]\d*)(?:-(?P<prerelease>(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+(?P<buildmetadata>[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?)$"
pullRefRegex = r"^refs/pull/(.*)/(.*)$"

with open(githubEnvPath, "a", encoding="utf-8") as githubEnv:
    versionsFile = os.getenv(
        "VERSIONS_FILE",
        Path(__file__).resolve().parents[2] / "versions.yaml")
    supportedReleases = get_supported_releases(versionsFile)
    latestStableRelease = next((
        (releaseChannel, releaseVersion.partition("+")[0])
        for releaseChannel, releaseVersion in supportedReleases
        if "-" not in releaseVersion.partition("+")[0]
    ), None)
    if latestStableRelease is None:
        raise ValueError(
            "Could not find the latest stable release in {}".format(versionsFile))
    latestChannel, latestVersion = latestStableRelease
    latestStableChannel = "LATEST_STABLE_CHANNEL={}".format(latestChannel)
    print("Setting: {}".format(latestStableChannel))
    githubEnv.write(latestStableChannel + "\n")
    latestStableVersion = "LATEST_STABLE_VERSION={}".format(latestVersion)
    print("Setting: {}".format(latestStableVersion))
    githubEnv.write(latestStableVersion + "\n")

    print("Setting: UPDATE_LATEST=false")
    githubEnv.write("UPDATE_LATEST=false" + "\n")
    print("Setting: UPDATE_CHANNEL=false")
    githubEnv.write("UPDATE_CHANNEL=false" + "\n")

    if gitRef is None:
        print(
            "This is not running in github, GITHUB_REF is null. Assuming a local build...")

        version = "REL_VERSION=edge"
        print("Setting: {}".format(version))
        githubEnv.write(version + "\n")

        chart = "CHART_VERSION=0.42.42-dev"
        print("Setting: {}".format(chart))
        githubEnv.write(chart + "\n")

        channel = "REL_CHANNEL=edge"
        print("Setting: {}".format(channel))
        githubEnv.write(channel + "\n")

        sys.exit(0)

    match = re.search(pullRefRegex, gitRef)
    if match is not None:
        print("This is pull request {}...".format(match.group(1)))

        version = "REL_VERSION=pr-{}".format(match.group(1))
        print("Setting: {}".format(version))
        githubEnv.write(version + "\n")

        chart = "CHART_VERSION=0.42.42-pr-{}".format(match.group(1))
        print("Setting: {}".format(chart))
        githubEnv.write(chart + "\n")

        channel = "REL_CHANNEL=edge"
        print("Setting: {}".format(channel))
        githubEnv.write(channel + "\n")

        sys.exit(0)

    match = re.search(tagRefRegex, gitRef)
    if match is not None:
        print("This is tagged as {}...".format(match.group("version")))

        if match.group("prerelease") is None:
            print("This is a full release...")

            version = "REL_VERSION={}".format(match.group("version"))
            print("Setting: {}".format(version))
            githubEnv.write(version + "\n")

            chart = "CHART_VERSION={}".format(match.group("version"))
            print("Setting: {}".format(chart))
            githubEnv.write(chart + "\n")

            channel = "REL_CHANNEL={}.{}".format(
                match.group("major"), match.group("minor"))
            print("Setting: {}".format(channel))
            githubEnv.write(channel + "\n")

            print("Setting: UPDATE_RELEASE=true")
            githubEnv.write("UPDATE_RELEASE=true" + "\n")

            releaseChannel = channel.removeprefix("REL_CHANNEL=")
            supportedVersion = dict(supportedReleases).get(releaseChannel)
            if match.group("version") == supportedVersion:
                print("Setting: UPDATE_CHANNEL=true")
                githubEnv.write("UPDATE_CHANNEL=true" + "\n")

            if match.group("version") == latestVersion:
                print("Setting: UPDATE_LATEST=true")
                githubEnv.write("UPDATE_LATEST=true" + "\n")
            sys.exit(0)

        else:
            print("This is a prerelease...")

            version = "REL_VERSION={}".format(match.group("version"))
            print("Setting: {}".format(version))
            githubEnv.write(version + "\n")

            chart = "CHART_VERSION={}".format(match.group("version"))
            print("Setting: {}".format(chart))
            githubEnv.write(chart + "\n")

            channel = "REL_CHANNEL={}".format(match.group("version"))
            print("Setting: {}".format(channel))
            githubEnv.write(channel + "\n")
            sys.exit(0)

    print("This is a normal build")
    version = "REL_VERSION=edge"
    print("Setting: {}".format(version))
    githubEnv.write(version + "\n")

    chart = "CHART_VERSION=0.42.42-dev"
    print("Setting: {}".format(chart))
    githubEnv.write(chart + "\n")

    channel = "REL_CHANNEL=edge"
    print("Setting: {}".format(channel))
    githubEnv.write(channel + "\n")
