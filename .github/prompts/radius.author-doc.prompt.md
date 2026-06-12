---
agent: agent
name: radius.author-doc
description: Draft a contributing or architecture doc from a topic and a code reference, in the repository standard format.
---

# Author a contributing or architecture doc

Read [.github/skills/radius-author-doc/SKILL.md](../skills/radius-author-doc/SKILL.md) and follow it end-to-end. The prose source of truth is [docs/contributing/authoring-contributing-docs.md](../../docs/contributing/authoring-contributing-docs.md).

Topic and starting code reference: ${input:topic:Topic and a starting code reference (file, package, command, or Make target)}

Draft the doc in the correct format, ground every path and command in real code, update the nearest index, and run `make spellcheck` before handing the draft off for human review.
