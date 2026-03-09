---
name: contributing-docs-updater
description: 'Update, create, review, and find gaps in contributor documentation including CONTRIBUTING.md, docs/contributing/, and docs/architecture/. Use when a task affects contributor setup, build, test, debug, review, or architecture guidance, or when a code review needs a documentation impact assessment.'
argument-hint: 'Describe what contributor docs to update, review, or create'
user-invocable: true
---

# Contributing Documentation Updater

Use this skill to maintain contributor-facing documentation that must stay aligned with the Radius repository.

This skill applies to:

- `CONTRIBUTING.md`
- `docs/contributing/`
- `docs/architecture/`
- Contributor-facing Copilot guidance in `.github/` when the requested change is specifically about contributor workflows

## When to Use

- Rewrite or improve existing contributor documentation
- Create new contributor guides for undocumented workflows
- Find gaps in documentation coverage and suggest what docs to create
- Review changes to contributor documents for accuracy and completeness
- During code reviews: determine if code changes require doc updates

Do not use this skill for general product documentation, end-user documentation, or code changes that have no contributor workflow impact.

## Core Principles

1. **Code first**: Verify commands, paths, and workflows against the repository before editing docs.
2. **Smallest correct scope**: Update the narrowest doc that fully covers the workflow. Do not spread one topic across multiple pages unless navigation requires it.
3. **Workflow over inventory**: Explain what contributors need to do, when they need to do it, and how to verify success.
4. **Cross-reference deliberately**: Update entry points and section indexes only when the doc structure changes.
5. **Review mode stays review-only**: In code review or doc review requests, report required doc changes unless the user explicitly asks you to edit them.

## Documentation Map

The documentation structure under `docs/contributing/` may change over time. Do not assume any specific sub-paths exist. Instead, discover the current layout at the start of every task:

1. List the directory tree under `docs/contributing/` to learn the current structure.
2. Read `CONTRIBUTING.md` and any top-level index files (e.g., `README.md` files, table-of-contents pages) to understand the navigation hierarchy.
3. Read `docs/architecture/` contents when the task involves architecture documentation.
4. Match the topic to the most appropriate existing location. If no existing section fits, propose the smallest new page or subsection and update the nearest parent index.

Always verify the actual directory structure before referencing, creating, or moving any doc.

## General Procedure

Follow this flow. After classifying the request, jump to the matching section under Modes for specific steps.

1. **Classify the request**: Decide whether this is a rewrite, new doc, gap analysis, doc review, or code review impact assessment.
2. **Discover the current doc structure**: Follow the Documentation Map procedure to learn the current layout before making any assumptions about paths.
3. **Verify the source of truth**: Read the relevant docs, then inspect the code, scripts, Make targets, configs, workflows, or directory structure they describe.
4. **Choose the target location**: Reuse an existing section when possible. If no existing section fits, propose the smallest new page or subsection.
5. **Make the smallest accurate change**: Prefer focused edits, concise steps, and validated examples.
6. **Update navigation when needed**: If you add a new page or change information architecture, update `CONTRIBUTING.md` and any relevant index or README files.
7. **Validate the result**: Confirm commands exist, paths are real, links resolve, and the doc matches current repo behavior.

## Modes

### Rewrite Existing Docs

Use this mode when the target page exists but is outdated, unclear, or incomplete.

1. Read the target doc and determine its intended audience and purpose.
2. Read the source code, configs, scripts, or workflows the doc describes.
3. Identify specific defects:
   - outdated commands or paths
   - missing prerequisites or verification steps
   - stale references to renamed files, targets, flags, or directories
   - content that has drifted from the repo's current workflow
4. Rewrite only the sections that need correction, following [Writing Guidelines](./references/writing-guidelines.md).
5. Re-check every command, path, and cross-reference you touched.

Expected output:

- Updated documentation with verified commands and paths
- Cross-reference updates only if structure changed

### Create New Docs

Use this mode when the repository supports a contributor workflow that is not documented anywhere appropriate.

1. Follow the Documentation Map procedure to discover the current structure and determine the correct directory.
2. Read adjacent docs in that section to match tone and structure.
3. Gather source material from the repository.
4. Draft a narrowly scoped page that explains:
   - when contributors need the workflow
   - prerequisites
   - exact steps
   - how to verify success
   - links to adjacent docs for prerequisite or follow-on tasks
5. Update navigation files only where discoverability requires it.

Expected output:

- New page or subsection in the correct contributor-doc location
- Matching navigation updates where needed

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

   | Gap | Suggested Doc | Location | Priority |
   |-----|---------------|----------|----------|
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

   | Change Type | Likely Doc Impact Area |
   |-------------|------------------------|
   | New CLI command or flag | CLI guides, first-commit walkthroughs |
   | Build system changes (Makefile, scripts) | Build and local dev guides |
   | New package or directory | Repo organization guides |
   | Test framework changes | Test execution guides |
   | Config file changes | Control plane or runtime config guides |
   | New prerequisites or tools | Prerequisites and setup guides |
   | API schema changes | Schema and API change guides |
   | Debugging workflow changes | Debugging guides |
   | New dev workflow or script | Contribution pathway index or new guide |
   | Architecture changes | `docs/architecture/` |
   | CI/CD workflow changes | Consider new guide |
   | TypeSpec/API spec changes | Schema guides, `docs/architecture/` |
   | Helm chart or deployment changes | Consider deployment guide |
   | Dev container changes | Prerequisites and setup guides |
   | Code generation changes | Code writing or generation guides |
   | Recipe changes | Consider recipe contribution guide |
   | Copilot instruction/skill changes | `.github/copilot-instructions.md` |

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

- the requested workflow could belong in multiple sections
- the user wants either a review or a direct edit and the intent is unclear
- the workflow appears to need a new page but the right parent section is not obvious

Otherwise, proceed without asking.

## Example Prompts

- `/contributing-docs-updater Review the changes in the contributor test docs for accuracy.`
- `/contributing-docs-updater Create a contributor guide for the local debug-start workflow.`
- `/contributing-docs-updater Do these Makefile changes require contributor doc updates?`
- `/contributing-docs-updater Find gaps in the contributor docs for TypeSpec and Swagger workflows.`

## Quality Checklist

Before finalizing any doc creation or update:

- [ ] All commands and paths verified against the codebase
- [ ] No broken internal links
- [ ] Follows the [Writing Guidelines](./references/writing-guidelines.md)
- [ ] Cross-references updated (CONTRIBUTING.md, index files, parent READMEs)
- [ ] No assumptions about reader's prior knowledge beyond stated prerequisites
- [ ] Multi-step procedures traced through the code, scripts, or Make targets they rely on

When operating in review-only modes, replace edits with precise findings and recommended doc changes.
