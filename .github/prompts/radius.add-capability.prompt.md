---
agent: agent
name: radius.add-capability
description: Walk through adding a new capability to the Agent Ex system — pick the asset type, author the primary doc, scaffold wrappers, and update the live files.
---

# Add a capability

Follow [docs/contributing/extending-agent-ex.md](../../docs/contributing/extending-agent-ex.md) end-to-end, using the [radius-add-capability](../agents/radius-add-capability.agent.md) agent's workflow. The conventions, budgets, and templates are in [docs/contributing/contributing-agent-assets.md](../../docs/contributing/contributing-agent-assets.md).

Capability to add: ${input:capability:Describe the new capability or workflow}

Decide where the capability lives (decision tree + two-of-four rule), author or extend its primary contributing doc, scaffold only the justified wrappers, and update the live files. Do not edit the planning docs `agent-ex-features.md` or `agent-ex-plan.md`. Validate against the Verification section and run `make spellcheck`.
