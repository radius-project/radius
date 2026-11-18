# Radius release notes

This directory contains the release notes for each Radius release. The release notes are written in Markdown and are included as the body of [each Radius release](https://github.com/radius-project/radius/releases).

## Release process

Refer to the [release process docs](../contributing/contributing-releases/README.md) for more information on how to create a new release.

## Release notes format

Each release note is a Markdown file named for the release tag, such as `v0.61.0-rc.1.md`, `v0.61.0.md`, or `v0.61.1.md`.

Refer to [template.md](./template.md) for the template to use when creating a new release note.

## Versions

The [Prepare Release workflow](https://github.com/radius-project/radius/actions/workflows/prepare-release.yaml) computes the version from the fixed policy, replaces the template placeholders, and includes the generated file in the release pull request. The workflow fails rather than accepting a manually chosen version that violates the policy.

The template retains the following marker so local break-glass preparation can find each version placeholder. Prepare Release removes the marker automatically.

```markdown
<!-- REMINDER TO UPDATE THE VERSION ABOVE AND DELETE THIS COMMENT -->
```

## Highlights

While the full changelog and release notes contain the generated change groups described below, the highlights section is a curated list of the most important changes in the release. This section should be written in a way that is easy for users to understand and digest. Talk to a PM if you need help determining what to include in the highlights section, and how to phrase it.

## Generated changelog and contributors

Prepare Release uses the pinned git-cliff configuration to update the canonical `CHANGELOG.md` and populate the release note's Breaking changes, New contributors, and Full changelog sections. Do not copy GitHub-generated notes or contributor lists into the file.

Conventional Commits select the [documented change groups](../contributing/contributing-pull-requests/README.md#3-use-a-conventional-commit-pull-request-title). Every commit in the selected release range is retained. Documentation, style, test, build, CI, ordinary chore, and other parsed conventional types without a specialized group appear under **Others**, as do nonconventional subjects and malformed conventional-looking headers without guessing their meaning. Others is the last change group before contributor sections. Breaking commits appear once under **Breaking changes** instead of their normal group; dependency updates retain their specialized group.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/), a human-readable changelog convention rather than a Conventional Commit requirement. `Added`, `Fixed`, and `Changed` use its suggested categories; `Breaking changes`, `Dependencies`, `Reverted changes`, `Others`, and contributor sections are Radius extensions. The canonical changelog keeps `Unreleased` first, then releases newest-first with ISO dates and version links. Each release note includes its date and version comparison link and ends with attribution that acknowledges these extensions and links the Radius version policy.

RC and final notes show New contributors only in their top-level section, not again in Full changelog. Patch notes retain their single generated New contributors section when first-time contributors are present. Explicit profile links, the comma-separated Contributors list, and contributor order come from git-cliff metadata; entries without a username are omitted.

Review the generated release pull request:

1. Verify the generated changelog groups and contributor list against the included commits.
2. Curate Highlights for user-facing importance.
3. Review and update Upgrading with any release-specific actions.
4. Mark the draft pull request ready only after the release plan and notes agree.

## Patch releases

Patch releases are uploaded to GitHub Releases and require patch release notes for the release workflow to execute. Prepare Release generates them from [template_patch.md](./template_patch.md) with the selected fixes and patch version.

Refer to [template_patch.md](./template_patch.md) for the template to use when creating a new patch release note.
