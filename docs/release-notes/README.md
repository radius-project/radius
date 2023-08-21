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

To generate the release notes:

1. Visit [Generate a new release](https://github.com/radius-project/radius/releases/new) in the radius repository.
   - _Note that you will not be creating the release through the UI, just generating the list of merged PRs. This could be automated in the future using the [GitHub API](https://docs.github.com/en/rest/commits/commits?apiVersion=2022-11-28#compare-two-commits)_
2. Dropdown the `Choose a tag` menu and manually enter the tag for the upcoming release (_e.g. `v0.21.0`_). Keep `Target` as 'main'.
3. Dropdown the `Previous tag` menu and select the tag for the previous minor release (_e.g. `v0.20.0`_). Don't select patch or RC releases.
4. Click `Generate release notes` to generate the markdown for the release notes.
5. Copy both the contents of `## What's Changed` and `## New Contributors` into their respective sections in the template (_make sure not to copy the headers, as they already exist in the template_).
6. Exit out of the window without publishing the release or saving a draft.
