---
name: radius-code-review
description: 'Review a GitHub pull request in the Radius repository and produce concise, line-accurate feedback. Use when asked to review a PR, generate review feedback for a PR, or stage review comments for a PR.'
argument-hint: 'Provide the PR number (and optionally repository owner/name) to review'
user-invocable: true
---

# Radius Code Review

The procedure for reviewing a specific Radius pull request end to end. The rubric for *what* a good review contains — principles, quality and test criteria, PR-title and documentation-impact expectations, and output format — lives in [`code-review.instructions.md`](../../instructions/code-review.instructions.md). This skill covers *how* to run the review: syncing to the latest PR head, resolving accurate line numbers, and delivering or staging comments. Keep working notes in the session and deliver the review through chat or the active PR review surface.

## When to Use

- Reviewing a specific pull request and producing structured feedback
- Drafting or staging PR review comments when the user asks

Do not use this skill for:

- General code authoring or refactoring
- Documentation-only drift checks (use `radius-contributing-docs-updater`)
- Reviewing local uncommitted changes (use the built-in `code-review` agent)

## Inputs

- **PR number** (required; ask if it is not provided and cannot be inferred)
- **Repository owner / name** (defaults to `radius-project/radius`)

## 1. Sync to the latest PR head

Never trust the current worktree, cached diffs, or a prior review pass.

- Run `gh pr view <pr-number> --json title,body,headRefName,headRefOid,baseRefName,baseRefOid,commits,files,comments,reviews` to collect PR metadata, changed files, prior discussion, and the head SHA.
- Run `git fetch`, then compare the local checkout to the remote head. Account for rebases and force-pushes: note when the head SHA, commit list, changed-file set, or merge base differs from anything you observed before.
- Recompute the merge base against the base branch and read the actual diff at the latest head before writing findings.

## 2. Review the changes

Apply the rubric in [`code-review.instructions.md`](../../instructions/code-review.instructions.md) and the matching language files in `.github/instructions/` (Go, Shell, Make, Docker, GitHub Workflows, Bicep, Markdown). Read complete functions or surrounding call sites when a hunk alone cannot prove or disprove a finding.

Prioritize high-confidence, actionable issues:

- Correctness bugs, missing error handling, regressions, race conditions, and flaky or broken tests
- Security, authorization, credential, injection, or supply-chain concerns
- API compatibility, migration, generated-code, or deployment issues
- Test gaps that leave changed behavior unprotected
- Documentation drift that misleads contributors

Skip pure preference, generic praise, and style already handled by automation.

## 3. Resolve accurate line numbers

Wrong line numbers are the most common defect in generated reviews. For every inline comment:

- Never type a line number from memory.
- Resolve it from the latest head with a deterministic lookup such as `git diff --unified=0 <merge-base>...<head>` plus a unique snippet search in the changed hunk.
- Confirm the line is on the `RIGHT` side of the diff and inside a changed hunk. If the issue concerns nearby unchanged code, attach to the closest changed line and say so in the comment body.
- Re-check line numbers after any rebase, force-push, or refreshed diff.

## 4. Validate before delivering

- Every finding is supported by the latest diff and surrounding code.
- Paths and line numbers point to the current head and appear in the diff.
- Comments do not duplicate existing reviewer feedback unless they add materially new evidence.
- Severity matches impact; do not block on advisory documentation drift.
- Drop speculative or low-confidence findings — returning no findings is better than noise.

## 5. Assess documentation impact

Follow the documentation-impact section of the rubric. Use the [`radius-contributing-docs-updater`](../radius-contributing-docs-updater/SKILL.md) skill; when the drift is directly caused by the PR, raise it as a normal finding with the specific doc path and required change.

## 6. Deliver or stage the review

- **Summary only**: reply in chat with the overall assessment and findings, using the rubric's output format.
- **Draft or stage comments**: if the environment exposes a PR review tool, stage comments in the pending review draft. Write each comment as the reviewer — direct and human-sounding, with no mention of a skill or AI. Do not submit the review unless the user explicitly asks and the tool supports it.
- **No PR review tool available**: provide the comments in chat in the rubric's output format so the user can paste them into GitHub.

Report one overall outcome: **No findings**, **Comments** (advisory), or **Request changes** (at least one blocking issue). Approve only when the user explicitly asks and you have authority in the active review surface.

## Example Prompts

- `/radius-code-review Review PR #1234`
- `/radius-code-review Review radius-project/radius#5678`
- `/radius-code-review Draft review comments for the active PR`

## Checklist

- [ ] Latest head fetched; merge base and changed files recomputed
- [ ] Rubric and language instructions applied
- [ ] Findings are specific, actionable, and high-confidence
- [ ] Cited paths and line numbers match the current diff
- [ ] Existing discussion checked to avoid duplicate comments
- [ ] Documentation impact assessed
