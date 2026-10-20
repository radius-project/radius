# Changelog

All notable changes to Radius are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Radius follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) with the version policy documented in the [release process](docs/contributing/contributing-releases/README.md).

## [Unreleased]

## [0.60.0] - 2026-08-19

### Added

- Added preview support for fully extensible, recipe-driven resource types in the `Radius.*` namespace.
- Added direct use of standard Bicep and Terraform modules as Radius Recipes.

### Changed

- Replaced SHA-1 with SHA-256 for internal resource identifiers, Terraform recipe state keys, ETags, and change-detection tokens while preserving compatibility with existing state.

For curated highlights, upgrade guidance, contributors, and the complete change list, see the [Radius v0.60.0 release notes](docs/release-notes/v0.60.0.md).

Release history before v0.60.0 remains available on [GitHub Releases](https://github.com/radius-project/radius/releases).

[Unreleased]: https://github.com/radius-project/radius/compare/v0.60.0...HEAD
[0.60.0]: https://github.com/radius-project/radius/releases/tag/v0.60.0
