# Radius release notes

This directory contains the release notes for each Radius release. The release notes are written in Markdown and are included as the body of [each Radius release](https://github.com/radius-project/radius/releases).

## Release process

Refer to the [release process docs](../contributing/contributing-releases/README.md) for more information on how to create a new release.

## Release notes format

Each release note is a Markdown file named `vX.Y.Z.md` where `X.Y.Z` is the semantic version of the release (_e.g. v0.21.0.md_).

Refer to [template.md](./template.md) for the template to use when creating a new release note.

## Versions

The template contains a few places where the placeholder version, `X.Y.Z`, needs to be updated. This version is determined by the release process, documented in the [release contribution docs](../contributing/contributing-releases/README.md).

 Check for the following comment, placed directly under any reference to the placeholder version. After updating the version make sure to delete the comment.

```markdown
<!-- REMINDER TO UPDATE THE VERSION ABOVE AND DELETE THIS COMMENT -->
```

## Highlights

While the full changelog and release notes contain every PR and commit that went into the release, the highlights section is a curated list of the most important changes in the release. This section should be written in a way that is easy for users to understand and digest. Talk to a PM if you need help determining what to include in the highlights section, and how to phrase it.

## Generating the full changelog (release notes) and new contributors

Within the template is the `## Full changelog` section, which is a complete list of commits merged since the last release.

To populate the release notes:

1. Open a pull request against `main` containing the final version update in `versions.yaml` and the draft release notes file.
2. Wait for the [Release Radius workflow](https://github.com/radius-project/radius/actions/workflows/release.yaml) to post the **Release Information** comment on the pull request.
3. Copy the contents under `## What's Changed` and `## New Contributors` from the comment into the corresponding sections in the release notes file. Do not copy the headings because they already exist in the template.
4. Commit and push the completed release notes to the same pull request.

## Patch releases

Patch releases will be uploaded to Github Releases and require patch release notes for the release workflow to execute. The patch release notes follow the same formatting as the original release notes, but only contain info about the changelog.

Refer to [template_patch.md](./template_patch.md) for the template to use when creating a new patch release note.
