## Announcing Radius vX.Y.Z
<!-- REMINDER TO UPDATE THE VERSION ABOVE AND DELETE THIS COMMENT -->

Today we're happy to announce the release of Radius vX.Y.Z. Check out the [highlights](#highlights) below, along with the [full changelog](#full-changelog) for more details.
<!-- REMINDER TO UPDATE THE VERSION ABOVE AND DELETE THIS COMMENT -->

We would like to extend our thanks to all the [new](#new-contributors) and existing contributors who helped make this release possible!

## Intro to Radius

If you're new to Radius, check out our website, [radapp.dev](https://radapp.dev), for more information. Also visit our [getting started guide](https://docs.radapp.dev/getting-started/) to learn how to install Radius and create your first app.

## Highlights

<!-- TALK TO THE PM TEAM ABOUT WHAT HIGHLIGHTS TO ADD HERE -->

## Breaking changes

<!-- ADD ANY BREAKING CHANGES HERE, IF ANY -->

## New contributors

<!-- PASTE THE OUTPUT OF THE GENERATED CONTRIBUTOR LIST HERE -->

## Upgrading to Radius vX.Y.Z
<!-- REMINDER TO UPDATE THE VERSION ABOVE AND DELETE THIS COMMENT -->

During our preview stage, an upgrade to Radius vX.Y.Z requires a full reinstallation of the Radius control-plane, rad CLI, and all Radius apps. Stay tuned for an in-place upgrade path in the future.
<!-- REMINDER TO UPDATE THE VERSION ABOVE AND DELETE THIS COMMENT -->

1. Visit the [Radius installation guide](https://docs.radapp.dev/getting-started/install/) to install the latest CLI, or download a binary below
1. Delete any environments you have created:
   ```bash
   rad env delete <env-name>
   ```
1. Uninstall the previous version of the Radius control-plane:
   ```bash
   rad uninstall kubernetes
   ```
1. Install the latest version of the Radius control-plane:
   ```bash
   rad install kubernetes
   ```

## Full changelog

<!-- PASTE THE OUTPUT OF THE GENERATED CHANGELOG HERE -->
