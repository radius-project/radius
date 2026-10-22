## Radius vX.Y.Z
<!-- REMINDER TO UPDATE THE VERSION ABOVE AND DELETE THIS COMMENT -->

This patch release includes the fixes listed in the [changelog](#changelog).

**Helm image pinning:** This release's Helm chart pins Radius component images to the full patch version. Restarting pods no longer picks up later patches implicitly; upgrade your CLI and run `rad upgrade kubernetes` to update the chart and images. Explicit channel-tag or image overrides remain respected; to adopt version-pinned defaults, clear tag overrides and restore image overrides to their default names.

## Changelog

<!-- PASTE THE OUTPUT OF THE GENERATED CHANGELOG HERE -->

---

The changelog is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/), with Radius-specific change groups and contributor sections. Radius follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) under its [version policy](https://github.com/radius-project/radius/blob/main/docs/contributing/contributing-releases/README.md).
