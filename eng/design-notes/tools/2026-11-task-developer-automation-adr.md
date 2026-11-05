# ADR: Use Task for local developer automation

## Context

Radius relies on Make and shell scripts for development automation, which prevents native Windows contributors from using the same workflows as macOS and Linux contributors. Windows should be a first-class development platform without requiring a WSL shell for ordinary builds and tests.

This decision only applies to developer environments; GitHub Actions will continue to use existing Make targets and shell scripts. Replacing all Make targets with Task could be a future decision.

## Decision

Use [Task](https://github.com/go-task/task) as a cross-platform alternative for local developer automation.

- Use Task-native declarative orchestration and direct tool invocation.
- When necessary logic cannot be expressed well in the Task declarative format, use Python scripts called by the Task target.
- No duplication: where Make and Task require the same behavior, have the make target reuse the Task implementation, or share the Python script between Make and Task.
- Scripts used only in GitHub Actions may remain shell scripts.
- Make targets used only in GitHub Actions do not need Task implementations.

This decision is not to replace all Make targets with Task targets; the task implementation will cover local developer automation activities that need to work on macOS, Windows, and Linux.

## Alternatives considered and not chosen

### Keep Make and require WSL on Windows

This preserves the existing tooling and avoids introducing another task runner. We did not choose it because requiring a WSL shell does not make native Windows a first-class development platform. Make remains available for existing workflows.

### Install Make on native Windows

This keeps a familiar entry point across platforms without requiring WSL. We did not choose it because installing Make alone does not make our shell-based recipes portable; we would still need to migrate their supporting shell scripts to Python. Given that migration work, we prefer a modern declarative tool like Task that improves local automation for contributors on all platforms, rather than only enabling the existing Make workflow on Windows.

### Build a repository-owned Go development CLI

A Go development CLI is a commonly used pattern, provides cross-platform execution, and uses a language already familiar to Radius contributors. We prototyped this approach and rejected it for this workflow because it required more custom imperative orchestration code to implement and maintain.

Task expresses commands, dependencies, and configuration declaratively, making routine automation easier to inspect and change without maintaining a command dispatcher and execution framework. Go remains suitable for more complex tools, but we prefer Task for development orchestration and small Python helpers only where necessary.

## Consequences

Contributors gain a consistent native build/test entry point and install Task alongside Go.

Make and Task coexist, so overlapping behavior must remain consistent. Windows developers would use Task, while Mac and Linux developers can choose Task or Make.

Some unit tests currently fail on Windows and will be fixed as part of the Task implementation. The fixes are simple.

The unit test GitHub workflow will be updated to include running the unit tests on Windows.

## Confirmation

Validate the developer workflow on Windows, macOS, and Linux, including supported amd64 and ARM64 hosts. Verify all-module and focused builds, tool flag forwarding, and failure propagation, and resolve platform-specific test failures.
