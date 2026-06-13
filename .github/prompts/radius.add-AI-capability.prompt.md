---
agent: agent
name: radius.add-AI-capability
description: Walk through adding a new AI capability to the Agent Ex system — pick the asset type, author the primary doc, scaffold wrappers, and update the live files.
---

# Add an AI capability

Follow [docs/contributing/extending-agent-ex.md](../../docs/contributing/extending-agent-ex.md) end-to-end, using the [radius-add-AI-capability](../agents/radius-add-AI-capability.agent.md) agent's workflow. The conventions, budgets, and templates are in [docs/contributing/contributing-agent-assets.md](../../docs/contributing/contributing-agent-assets.md).

AI capability to add: ${input:capability:Describe the new AI capability or workflow}

Decide where the AI capability lives (decision tree + two-of-four rule), author or extend its primary contributing doc, scaffold only the justified wrappers, and update the live files. Do not edit the planning docs `agent-ex-features.md` or `agent-ex-plan.md`. Validate against the Verification section and run `make spellcheck`.
