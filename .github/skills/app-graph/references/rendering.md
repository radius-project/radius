# Rendering Conventions

These conventions are a verbatim port of
`radius-project/github-extension` `src/content/graph-renderer.ts`. The
template file already encodes them — this file is for reference only.

## Library versions

The template loads from CDN:

- `cytoscape` v3 (`https://unpkg.com/cytoscape@3/dist/cytoscape.min.js`)
- `cytoscape-dagre` (`https://unpkg.com/cytoscape-dagre@2/cytoscape-dagre.js`)
- `dagre` (peer dep of cytoscape-dagre, also from unpkg)
- `cytoscape-navigator` v1.4 (powers the optional minimap — additive
  feature, see [interactive controls](#interactive-controls-additive))

## Layout

```js
{
  name: 'dagre',
  rankDir: 'TB',
  nodeSep: 60,
  rankSep: 80,
  edgeSep: 20,
  padding: 48,
  animate: false,
}
```

## Cytoscape core options

```js
{
  userZoomingEnabled: true,
  userPanningEnabled: true,
  boxSelectionEnabled: false,
  autoungrabify: true,
  minZoom: 0.3,
  maxZoom: 3,
}
```

After init, call `cy.resize(); cy.fit(cy.elements(), 48); cy.center();`
inside two nested `requestAnimationFrame` calls so the graph re-fits
once the container has its final size.

## Node style

```js
{
  selector: 'node',
  style: {
    label: 'data(label)',
    'text-valign': 'center',
    'text-halign': 'center',
    'font-size': '12px',
    'font-family': '-apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", Helvetica, Arial, sans-serif',
    color: '#1f2328',
    'background-color': 'data(bgColor)',
    'border-color': 'data(borderColor)',
    'border-width': 'data(borderWidth)',
    shape: 'roundrectangle',
    width: 140,
    height: 55,
    'text-wrap': 'wrap',
    'text-max-width': '120px',
  },
}
```

## Edge style

```js
{
  selector: 'edge',
  style: {
    width: 2,
    'line-color': '#8c959f',
    'target-arrow-color': '#8c959f',
    'target-arrow-shape': 'triangle',
    'curve-style': 'unbundled-bezier',
    'control-point-distances': [40],
    'control-point-weights': [0.5],
    'arrow-scale': 0.8,
  },
}
```

## Diff color tables (Primer)

Border colors and widths:

| Status    | Border    | Width |
| --------- | --------- | ----- |
| added     | `#1a7f37` | 3     |
| removed   | `#cf222e` | 3     |
| modified  | `#9a6700` | 3     |
| unchanged | `#57606a` | 2     |

Background fills:

| Status    | Fill      |
| --------- | --------- |
| added     | `#dafbe1` |
| removed   | `#ffebe9` |
| modified  | `#fff8c5` |
| unchanged | `#f6f8fa` |

In single-graph mode (this skill's default), every node has diff status
`unchanged`. The diff fill/border are then replaced by per-resource-type
colors and the label is prefixed with a type icon — see
[visual-style.md](visual-style.md). In a future diff mode the diff
status fill/border wins so the diff signal stays dominant.

## Node label

`label = `${resource.name}\n${shortType}`` where
`shortType = resource.type.split('/').pop() ?? resource.type`.

## Edge rule

Iterate every resource. For each `Outbound` connection, add an edge from
the owning resource to the connection's `id` **only if** the target `id`
appears in `application.resources`. Skip otherwise. Edge id is
`${source}-->${target}`.

## Popup behavior

On node tap: read the node's rendered position, anchor a floating div
at `position + (10, 10)`, show:

1. Title = `resource.name`
2. Subtitle = `resource.type`
3. `📄 Source code` link to `resource.codeReference` (only if present
   and parseable). For the standalone HTML viewer, the link target is a
   `file://` URL or plain path — no GitHub context is available.
4. `📐 App definition` link to `sourceFile#L<appDefinitionLine>`.
5. `×` close button.

Close on outside click, ESC, or close-button click.

Critically: the popup div must call `stopPropagation` on `mousedown`,
`mouseup`, `touchstart`, `touchend`, `pointerdown`, `pointerup` so
Cytoscape's container-walking handler does not intercept link clicks.

## Tap-on-background closes popup

`cy.on('tap', e => { if (e.target === cy) closeGraphPopup(); })`.

## Interactive controls (additive)

These features go beyond the extension's built-in renderer. They are
opt-in via DOM interaction only — no programmatic API change is needed
in the artifact. None of them touch the diff color tables, the dagre
layout, or the edge construction rule. Implementation lives entirely in
`template/app-graph.html.tmpl`.

| Feature              | UI surface                  | Cytoscape primitive                                             |
| -------------------- | --------------------------- | --------------------------------------------------------------- |
| Hover tooltip        | `#hover-tip` floating div   | `cy.on('mouseover'/'mouseout'/'mousemove', 'node', ...)`        |
| Neighbor highlight   | `.faded` cytoscape class    | `node.closedNeighborhood()` + `cy.elements().difference(...)`   |
| Search filter        | `#search` input             | `node.toggleClass('filtered-out', ...)` on input event          |
| Legend type toggle   | `.radius-graph-legend-item` | Same `filtered-out` class — both filters compose via `OR`       |
| Fit / Zoom / Reset   | `#controls` button group    | `cy.fit(...)`, `cy.zoom(...)`                                   |
| Minimap              | `#minimap` div              | `cy.navigator({...})` from `cytoscape-navigator@1.4`            |

### Visibility model

Two filter sources — search and legend toggle — share a single
`filtered-out` class on each node. An edge is `filtered-out` iff either
endpoint is. Hovered-node neighbor highlighting uses a separate `faded`
class on non-neighbors so the two effects compose without interfering.

### Reset

The Reset button (`⟲`) clears `#search`, clears all type toggles, and
calls `cy.fit(cy.elements(), 48)`. It does NOT close any open popup —
ESC or outside click handles that.

### Failure modes

If `cytoscape-navigator` fails to load (offline mirror, blocked CDN),
the script falls back to `display: none` on `#minimap` so the canvas
still works. The hover/search/controls/legend features depend only on
`cytoscape` + `cytoscape-dagre`, which the renderer requires anyway.
