---
name: radius-contributing-docs-updater
description: 'Assess contributor docs: find which docs are MISSING or STALE across CONTRIBUTING.md, docs/contributing/, and docs/architecture/, or judge whether a code change needs doc updates. Produces prioritized findings and routes the work — it does not write docs itself. To create a doc use radius-author-doc; to fix a drifted doc use radius-update-doc.'
argument-hint: 'Describe the docs to audit, or the code change to assess for doc impact'
user-invocable: true
---

# Contributing docs — assess & route

Audit contributor documentation and decide what work is needed. This skill **finds** missing or stale docs and **assesses** whether a code change needs doc updates. It does **not** write or patch docs — it hands that off (see the table below).

## Which doc skill?

| You want to…                                                         | Use                                                                          |
|----------------------------------------------------------------------|------------------------------------------------------------------------------|
| **Find** missing or stale docs, or assess a code change's doc impact | **this skill**                                                               |
| **Create** a new contributing doc                                    | [radius-author-doc](../radius-author-doc/SKILL.md)                           |
| **Fix** an existing doc that drifted from code                       | [radius-update-doc](../radius-update-doc/SKILL.md)                           |
| **Diagram** a subsystem / write an architecture doc                  | [radius-architecture-documenter](../radius-architecture-documenter/SKILL.md) |

This skill covers `CONTRIBUTING.md`, `docs/contributing/`, and `docs/architecture/`.

## When to Use

- **Gap analysis**: find what contributor docs are missing or stale and propose what to create.
- **Code-review doc impact**: given a code change, decide whether (and which) contributor docs must change.
- **Doc review**: review proposed doc changes for accuracy and completeness, reporting findings only.

Do not use this skill to author or edit docs (delegate per the table above), or for end-user/product documentation.

## Core Principles

1. **Code first**: Verify commands, paths, and workflows against the repository before judging a doc.
2. **Smallest correct scope**: Point to the narrowest doc that fully covers the workflow. Do not spread one topic across multiple pages unless navigation requires it.
3. **Workflow over inventory**: Judge docs by whether they tell contributors what to do, when, and how to verify success.
4. **Cross-reference deliberately**: Flag entry points and section indexes that need updating only when the doc structure changes.
5. **Findings, not edits**: Report required doc changes and route them; do not author or patch docs yourself.

## Documentation Map

The documentation structure under `docs/contributing/` may change over time. Do not assume any specific sub-paths exist. Instead, discover the current layout at the start of every task:

1. List the directory tree under `docs/contributing/` to learn the current structure.
2. Read `CONTRIBUTING.md` and any top-level index files (e.g., `README.md` files, table-of-contents pages) to understand the navigation hierarchy.
3. Read `docs/architecture/` contents when the task involves architecture documentation.
4. Match the topic to the most appropriate existing location. If no existing section fits, propose the smallest new page or subsection and update the nearest parent index.

Always verify the actual directory structure before referencing, creating, or moving any doc.

## General Procedure

1. **Classify the request**: gap analysis, code-review doc impact, or doc review.
2. **Discover the current doc structure**: follow the Documentation Map procedure before assuming any paths.
3. **Verify against the source of truth**: read the relevant docs, then inspect the code, scripts, Make targets, configs, or workflows they describe.
4. **Produce findings**: a prioritized gap list, a yes/no impact verdict with target docs, or review feedback — see the matching mode below.
5. **Route the work**: hand each "create" to [radius-author-doc](../radius-author-doc/SKILL.md) and each "fix" to [radius-update-doc](../radius-update-doc/SKILL.md). This skill stops at findings; it does not edit docs.

## Modes

> To **create** or **fix** a doc, stop and delegate: [radius-author-doc](../radius-author-doc/SKILL.md) creates, [radius-update-doc](../radius-update-doc/SKILL.md) fixes. The modes below produce findings only.

### Gap Analysis

Use this mode when the user wants to know what contributor docs are missing or stale.

1. Inventory the existing contributor docs and summarize what each section already covers.
2. Scan the repository for contributor-relevant workflows, especially:
   - setup and prerequisites
   - build, test, debug, and generation commands
   - CLI and control plane workflows
   - schema, API, and code generation changes
   - release, PR, triage, and automation processes
3. Cross-reference documented workflows against actual repository capabilities.
4. Report gaps in this format:

   | Gap                           | Suggested Doc  | Location      | Priority        |
   |-------------------------------|----------------|---------------|-----------------|
   | Description of what's missing | Proposed title | Proposed path | High/Medium/Low |

5. Prioritize using this rubric:
   - High: blocks or misleads new contributors
   - Medium: slows common contributor workflows
   - Low: helpful but not required for contributor success

Expected output:

- Gap table with specific, actionable doc proposals
- Brief note on why each high-priority gap matters

### Review Doc Changes

Use this mode when contributor docs changed and the request is for review, not direct editing.

1. Read the changed files and identify what contributor workflow they describe.
2. Verify commands, paths, examples, and prerequisites against the codebase.
3. Check for omissions that would cause a contributor to fail or get stuck.
4. Check consistency with surrounding docs and [Writing Guidelines](./references/writing-guidelines.md).
5. Check whether links and navigation entries still make sense.
6. Provide review feedback using the repo's code review format.

Review focus:

- incorrect commands or flags
- wrong file paths or stale directory names
- missing prerequisites, environment assumptions, or validation steps
- structural changes that were not reflected in parent indexes

### Code Review Doc Impact Assessment

Use this mode during code review when the changed code might require contributor doc updates.

1. Discover the current doc structure by following the Documentation Map procedure.
2. Categorize the change by contributor impact area:

   | Change Type                              | Likely Doc Impact Area                  |
   |------------------------------------------|-----------------------------------------|
   | New CLI command or flag                  | CLI guides, first-commit walkthroughs   |
   | Build system changes (Makefile, scripts) | Build and local dev guides              |
   | New package or directory                 | Repo organization guides                |
   | Test framework changes                   | Test execution guides                   |
   | Config file changes                      | Control plane or runtime config guides  |
   | New prerequisites or tools               | Prerequisites and setup guides          |
   | API schema changes                       | Schema and API change guides            |
   | Debugging workflow changes               | Debugging guides                        |
   | New dev workflow or script               | Contribution pathway index or new guide |
   | Architecture changes                     | `docs/architecture/`                    |
   | CI/CD workflow changes                   | Consider new guide                      |
   | TypeSpec/API spec changes                | Schema guides, `docs/architecture/`     |
   | Helm chart or deployment changes         | Consider deployment guide               |
   | Dev container changes                    | Prerequisites and setup guides          |
   | Code generation changes                  | Code writing or generation guides       |
   | Recipe changes                           | Consider recipe contribution guide      |
   | Copilot instruction/skill changes        | `.github/copilot-instructions.md`       |

3. Locate the specific doc that covers the impacted area by searching the discovered structure.
4. Assess whether contributor behavior changes. An update is usually needed if the change:
   - affects setup, build, test, debug, release, or review workflows
   - introduces a new tool, target, flag, config, or contributor-facing file
   - changes or removes behavior already described in existing docs
5. If an update is needed, specify:
   - which doc to update (using the actual path discovered earlier)
   - which section to modify or add
   - what contributor-facing behavior changed
6. If no update is needed, state why not.

Expected output:

- Clear yes or no on doc impact
- Specific target docs and section-level guidance when the answer is yes

## Decision Rules

Ask a clarifying question only when one of these is ambiguous:

- the workflow could belong in multiple sections
- the missing workflow appears to need a new page but the right parent section is not obvious

Otherwise, proceed without asking.

## Example Prompts

- `/radius-contributing-docs-updater Find gaps in the contributor docs for TypeSpec and Swagger workflows.`
- `/radius-contributing-docs-updater Do these Makefile changes require contributor doc updates?`
- `/radius-contributing-docs-updater Review the changes in the contributor test docs for accuracy.`

To write a missing doc, use `/radius.author-doc`; to fix a drifted doc, use the [radius-update-doc](../radius-update-doc/SKILL.md) skill.

## Quality Checklist

Before delivering findings:

- [ ] Current doc structure discovered (no assumed paths)
- [ ] Every referenced command, path, and link verified against the codebase
- [ ] Gaps and impacts are specific and prioritized, with concrete target docs
- [ ] Each recommended change is routed to the right skill (create → radius-author-doc, fix → radius-update-doc)
- [ ] Findings cite the source code, scripts, or Make targets that justify them
