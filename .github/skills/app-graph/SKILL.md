---
name: app-graph
description: >
  Generate an interactive application graph visualization from a Radius
  app.bicep file. Use when asked to visualize a Radius application,
  generate an app graph, build an app graph, render the graph for an app,
  show the application graph, or create an interactive graph. Produces
  a self-contained HTML viewer (Cytoscape + dagre) that mirrors the
  radius-project/github-extension graph renderer, opens it in the default
  browser, and emits an inline mermaid preview in chat.
---

# Radius Application Graph

Use this skill to render an interactive application graph from a Radius
`app.bicep` file. The skill compiles the Bicep, invokes `rad graph build`,
and produces a single self-contained `app-graph.html` whose renderer is
ported verbatim from `radius-project/github-extension`
(`src/content/graph-renderer.ts` + `src/content/graph-navigation.ts`).

## Output Format

Your entire visible response must follow this exact sequence. No extra
headings, no analysis preamble.

1. Say: I will generate an application graph for `<app-name>`.
2. Show exactly these lines as a single blockquote:
   > Compile app.bicep with the bicep CLI.
   > Build the static graph artifact with `rad graph build`.
   > Render the interactive HTML viewer.
   > Generate an inline mermaid preview.
3. Say: Here is an inline preview of the graph:
4. Output a single ```mermaid fenced code block matching the rules in
   [mermaid.md](references/mermaid.md).
5. Say: A full interactive viewer has been written to `./app-graph.html`.
6. Say: Opening it in your default browser now.
7. Open the file (see Internal Workflow step 6).
8. Say: Click any node in the viewer to see source-code and
   app-definition links.

That is the COMPLETE chat response.

## Internal Workflow (do NOT show these steps to the user)

1. Locate the Bicep file. Default search order: `./app.bicep`,
   `./.radius/app.bicep`. If neither exists, ask the user for the path.
2. Verify the `rad` CLI is available and supports `rad graph build`.
   Run `rad graph build --help`. If the subcommand is missing, instruct
   the user to install or build `rad` from
   `radius-project/radius` (`features/radius-graph` branch until the
   change merges to `main`). Do NOT auto-build.
3. Verify the `bicep` CLI is on PATH. Try `bicep --version`, then
   `az bicep version` as fallback. If neither works, instruct the user
   to install Bicep and abort.
4. Run `rad graph build --bicep <file> --output ./.radius/static/app.json`.
   The CLI compiles Bicep, parses resources/connections, computes
   diff hashes, and writes the `StaticGraphArtifact` JSON.
   Read [artifact-path.md](references/artifact-path.md) for details.
5. Read the JSON, then render the HTML viewer:
   a. Read [`template/app-graph.html.tmpl`](template/app-graph.html.tmpl).
   b. Replace the literal token `__GRAPH_DATA__` with the file
      contents of `app.json` (substitute as a JSON literal — do NOT
      wrap in quotes).
   c. Write the result to `./app-graph.html`.
6. Open the HTML file in the user's default browser:
   - Windows: `Start-Process .\app-graph.html`
   - macOS: `open ./app-graph.html`
   - Linux: `xdg-open ./app-graph.html`
7. Emit the mermaid preview per [mermaid.md](references/mermaid.md). One
   node per resource, edges from each resource's `Outbound` connections
   only when the target resource is also present, node text =
   `<name><br/><shortType>`, no diff coloring (single-graph mode).

## Renderer Conventions

Read [rendering.md](references/rendering.md) for the exact Cytoscape +
dagre options, the Primer color tables, and the edge construction rule.
Read [visual-style.md](references/visual-style.md) for the resource-type
icon + color palette used to theme nodes and the legend in single-graph
mode.

## Schema

Read [schema.md](references/schema.md) for the `StaticGraphArtifact` and
`ApplicationGraphResource` shape consumed by the HTML viewer.

## Validation Checklist

Before saying "Opening it in your default browser now", verify ALL:

- [ ] `./app-graph.html` exists and is non-empty.
- [ ] The JSON substituted into the template is valid (parses).
- [ ] Every resource in `application.resources` has `id`, `name`, `type`.
- [ ] The mermaid block only references resource IDs that also appear
      as mermaid node declarations (no dangling edges).
- [ ] The opener command used matches the host OS.

## Guardrails

- The renderer in `template/app-graph.html.tmpl` is a port of
  `graph-renderer.ts` + `graph-navigation.ts`. The layout, popup
  behavior, edge rule, and Primer color tables MUST stay verbatim. The
  resource-type icon/color palette ([visual-style.md](references/visual-style.md))
  is an additive extension — extend it for new resource types instead
  of rewriting the renderer.
- Do NOT inline a screenshot in chat in place of the mermaid block — the
  user wants a structural preview, not a rasterized one.
- Do NOT push to or create any orphan branch from this skill. The
  `--orphan-branch` flag is reserved for the CI workflow.
- Do NOT prompt the user before opening the HTML — the Output Format
  already announces the open step.
- If `rad graph build` fails, surface the stderr verbatim and stop.
  Do NOT attempt to construct the JSON manually.
- The HTML viewer is single-graph only. Diff coloring is preserved in
  the ported code but unused; do NOT add UI to load a second artifact.
  In single-graph mode every node uses the per-resource-type fill +
  border + icon from `TYPE_STYLES`; the diff palette is reserved for
  when a future diff mode lands.
