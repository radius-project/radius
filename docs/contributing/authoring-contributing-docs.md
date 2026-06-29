<!-- markdownlint-disable MD060 -->
# Authoring contributing and architecture docs

## Purpose

This document defines the two standard formats for documentation in this repository and shows one annotated, real example of each:

- **Contributing docs** (`docs/contributing/`, [CONTRIBUTING.md](../../CONTRIBUTING.md)) answer *"how do I do X?"* and follow: **Purpose → Prerequisites → Steps → Verification → Troubleshooting**.
- **Architecture docs** ([docs/architecture/](../architecture/)) answer *"how does X work?"* and follow: **Entry points → Key packages → Flow → Change-safety**.

A consistent format means an agent (or a human) always knows where to find the prerequisites, the verification step, or the change-safety notes — no matter who wrote the doc. This document follows the contributing-doc format it prescribes, so it doubles as a worked example.

For naming, file-size budgets, and CI gates that also apply to docs, see [contributing-agent-assets.md](./contributing-agent-assets.md). To turn a doc into a new capability (with optional skill/prompt/agent wrappers), see [extending-agent-ex.md](./extending-agent-ex.md).

## Prerequisites

- A topic and a starting **code reference** — a file, package, command, or Make target the doc will describe. Docs are grounded in real code, not speculation.
- Access to the repository so you can verify every command, path, and flag.
- Familiarity with the [file-strategy rule](./contributing-agent-assets.md#the-file-strategy-rule): capability lives in docs; wrappers link back to docs.

## Steps

### 1. Decide which format applies

- Writing down *how to perform a task* → **contributing doc**.
- Explaining *how a subsystem works* → **architecture doc**.

A capability's primary backing doc is always a contributing doc; architecture docs are reference material that contributing docs link to for depth.

### 2. Choose the location

Reuse an existing section when one fits. Discover the current layout first — list `docs/contributing/` and read the nearest index/`README.md` — then place the doc in the narrowest section that fully covers the topic. If none fits, create the smallest new page and link it from the nearest parent index.

### 3. Write a contributing doc in the standard format

Use these five sections in order:

| Section | Contents |
|---|---|
| **Purpose** | What the doc covers, who it's for, and why. One short paragraph. |
| **Prerequisites** | Tools, access, and prior steps required before starting. |
| **Steps** | Numbered, verified actions. Each command and path must be real. |
| **Verification** | How the contributor confirms success. |
| **Troubleshooting** | Common failures and their fixes. |

**Annotated example**: [contributing-code/contributing-code-debugging/radius-os-processes-debugging.md](./contributing-code/contributing-code-debugging/radius-os-processes-debugging.md) opens with an **Overview** (its Purpose), lists concrete **Prerequisites** (a cluster, tools, credentials), walks an ordered **Debugging Workflow** (its Steps), and closes with a **Troubleshooting** section. When you adapt it, keep verification explicit — state the exact command output or UI state that confirms success.

### 4. Write an architecture doc in the standard format

Use these four sections:

| Section | Contents |
|---|---|
| **Entry points** | The file + symbol where execution or a request begins. |
| **Key packages** | A table of packages and their single responsibility. |
| **Flow** | One representative end-to-end flow as a Mermaid diagram. |
| **Change-safety** | What tests to run and which components move together. |

**Annotated example**: [docs/architecture/ucp.md](../architecture/ucp.md) names its **Entry points** with file links (`cmd/ucpd/cmd/root.go`, `pkg/ucp/server/server.go`), tabulates **Core Packages** and their responsibilities, explains the request **flow**, includes a Mermaid package dependency graph, and ends with **Change This Safely** (packages that move together plus the suggested `go test` scope). Every path it cites resolves to a real file.

### 5. Ground every reference in code

- Link to real files and symbols; never invent a path.
- Verify each command and flag by running it or reading the source.
- Links in `docs/contributing/` are relative to the file: repo-root files are `../../`, architecture docs are `../architecture/`, sibling contributing docs are `./`.

### 6. Update navigation

If you added a new page, link it from the nearest index — a section `README.md`, [CONTRIBUTING.md](../../CONTRIBUTING.md), or [docs/architecture/README.md](../architecture/README.md) — and, when it backs a capability, add a row to the capability index (see [extending-agent-ex.md](./extending-agent-ex.md)).

### Doing this with an agent

You can follow these steps manually, or ask an agent to draft the doc from a topic and a code reference. The [radius-author-doc](../../.github/skills/radius-author-doc/SKILL.md) skill encodes this authoring workflow; this document is the prose source of truth it links back to. Either way, a human reviews the draft before merge.

## Verification

A doc is ready when:

- It uses the correct format for its type (all five contributing sections, or all four architecture sections).
- Every command, path, flag, and link resolves to something real.
- A reviewer can merge it after one round of edits, not a rewrite.
- `cspell` passes (`make spellcheck`).

## Troubleshooting

- **The doc spans two formats.** It is probably two docs. Split the "how it works" material into an architecture doc and link to it from the contributing doc.
- **No obvious home for the doc.** Create the smallest new page under the closest existing section and update that section's index, rather than expanding an unrelated page.
- **A referenced command changed.** Update the doc in the same PR as the code change; the docs-drift code-review instructions exist to catch this.
- **Spellcheck flags a real technical term.** Add it to [.cspellignore](../../.cspellignore).
