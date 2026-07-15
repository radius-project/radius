# Contributing Dockerfiles

## Purpose

This is the primary doc for writing and modifying the Dockerfiles that package the Radius control-plane services and tools into container images. It is reference material for anyone editing an image build. The detailed, rule-by-rule conventions live in the [Docker instruction file](../../../../.github/instructions/docker.instructions.md), which Copilot applies automatically to any `Dockerfile` you edit; this doc gives the map of where the Dockerfiles live and how they are built.

## Where these files live

- `deploy/images/<service>/Dockerfile` — one directory per shipped image (`applications-rp`, `dynamic-rp`, `ucpd`, `controller`, `bicep`, and others). Some services also carry a `Dockerfile.mariner` variant.
- The image builds are wired through [`build/docker.mk`](../../../../build/docker.mk) and invoked with `make docker-build` / `make docker-push` (see [building the repo](../contributing-code-building/README.md)).

## Conventions

Follow the [Docker instruction file](../../../../.github/instructions/docker.instructions.md). Its emphasis for Radius:

- **Multi-stage builds** — compile in a build stage and copy only the resulting binary into a minimal runtime stage.
- **Minimal, non-root runtime** — prefer distroless/minimal base images and run as a non-root user.
- **Deterministic layers** — order instructions for cache reuse and copy only what each stage needs.

## Verification

- `make docker-build` builds your image locally without errors.
- The image runs and the service starts (see [running & debugging locally](../contributing-code-debugging/radius-os-processes-debugging.md)).

## Related docs

- [Building the repo](../contributing-code-building/README.md) — the `make docker-*` targets.
- [Documentation index](../../README.md) — every contributing doc.
