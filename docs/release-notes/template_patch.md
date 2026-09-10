## Radius vX.Y.Z
<!-- REMINDER TO UPDATE THE VERSION ABOVE AND DELETE THIS COMMENT -->

This patch release includes the fixes listed in the [changelog](#changelog).

**Helm image pinning:** This release's Helm chart pins Radius component images to the full patch version. Restarting pods no longer picks up later patches implicitly; upgrade your CLI and run `rad upgrade kubernetes` to update the chart and images. Explicit channel-tag or image overrides remain respected; clear them to adopt version-pinned defaults.

## Changelog

<!-- PASTE THE OUTPUT OF THE GENERATED CHANGELOG HERE -->
