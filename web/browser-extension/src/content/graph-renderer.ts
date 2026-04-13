// Cytoscape.js graph renderer with cytoscape-dagre DAG layout.
// Renders ApplicationGraphResource arrays as interactive node graphs
// with diff status coloring and node tap events for navigation popups.

import type { ApplicationGraphResource, DiffStatus, GraphDiff } from '../shared/graph-types.js';
import { getResourceDiffStatus } from './graph-diff.js';
import { showGraphPopup, closeGraphPopup, type PopupContext } from './graph-navigation.js';

// Cytoscape is bundled by esbuild; imported as a module.
import cytoscape from 'cytoscape';
import dagre from 'cytoscape-dagre';

// Register the dagre layout extension.
cytoscape.use(dagre);

/** Primer diff colors for node borders. */
const DIFF_COLORS: Record<DiffStatus, string> = {
  added: '#1a7f37',    // --color-success-fg
  removed: '#cf222e',  // --color-danger-fg
  modified: '#9a6700', // --color-attention-fg
  unchanged: '#656d76', // --color-neutral-emphasis
};

const DIFF_BORDER_WIDTH: Record<DiffStatus, number> = {
  added: 3,
  removed: 3,
  modified: 3,
  unchanged: 1,
};

export interface RenderGraphOptions {
  container: HTMLElement;
  resources: ApplicationGraphResource[];
  diff: GraphDiff | null;
  context: PopupContext;
}

/**
 * Render an interactive application graph using Cytoscape.js.
 * Returns the Cytoscape instance for cleanup.
 */
export function renderGraph(options: RenderGraphOptions): cytoscape.Core {
  const { container, resources, diff, context } = options;

  // Convert resources to Cytoscape elements.
  const elements = buildCytoscapeElements(resources, diff);

  const cy = cytoscape({
    container,
    elements,
    layout: {
      name: 'dagre',
      rankDir: 'TB',
      nodeSep: 60,
      rankSep: 80,
      edgeSep: 20,
      padding: 48,
      animate: false,
    } as cytoscape.LayoutOptions,
    style: [
      {
        selector: 'node',
        style: {
          label: 'data(label)',
          'text-valign': 'bottom',
          'text-halign': 'center',
          'text-margin-y': 8,
          'font-size': '11px',
          'font-family': '-apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", Helvetica, Arial, sans-serif',
          'background-color': '#ffffff',
          'border-color': 'data(borderColor)',
          'border-width': 'data(borderWidth)',
          shape: 'roundrectangle',
          width: 140,
          height: 50,
          'text-wrap': 'wrap',
          'text-max-width': '130px',
        },
      },
      {
        selector: 'edge',
        style: {
          width: 2,
          'line-color': '#d0d7de',
          'target-arrow-color': '#d0d7de',
          'target-arrow-shape': 'triangle',
          'curve-style': 'bezier',
          'arrow-scale': 0.8,
        },
      },
      {
        selector: 'node:active',
        style: {
          'overlay-opacity': 0.1,
        },
      },
    ],
    userZoomingEnabled: true,
    userPanningEnabled: true,
    boxSelectionEnabled: false,
    autoungrabify: true,
    minZoom: 0.3,
    maxZoom: 3,
  });

  const fitGraph = () => {
    cy.resize();
    cy.fit(cy.elements(), 48);
    cy.center();
  };

  // Cytoscape can initialize before the container has fully settled in the GitHub page layout.
  // Re-fit after the first paint so the graph uses the available canvas instead of clustering in a corner.
  requestAnimationFrame(() => {
    fitGraph();
    requestAnimationFrame(fitGraph);
  });

  // Bind node tap events for popup navigation.
  cy.on('tap', 'node', (event) => {
    const node = event.target;
    const resourceId = node.data('resourceId') as string;
    const resource = resources.find((r) => r.id === resourceId);
    if (!resource) return;

    const diffStatus = diff ? getResourceDiffStatus(resourceId, diff) : 'unchanged';
    const renderedPosition = node.renderedPosition();

    void showGraphPopup(resource, diffStatus, context, container, {
      x: renderedPosition.x + 10,
      y: renderedPosition.y + 10,
    });
  });

  // Close popup on canvas tap.
  cy.on('tap', (event) => {
    if (event.target === cy) {
      closeGraphPopup(container);
    }
  });

  return cy;
}

/**
 * Convert ApplicationGraphResource array to Cytoscape element definitions.
 */
function buildCytoscapeElements(
  resources: ApplicationGraphResource[],
  diff: GraphDiff | null,
): cytoscape.ElementDefinition[] {
  const elements: cytoscape.ElementDefinition[] = [];

  // Add nodes.
  for (const resource of resources) {
    const diffStatus: DiffStatus = diff
      ? getResourceDiffStatus(resource.id, diff)
      : 'unchanged';

    // Build display label: name + short type + optional image tag.
    const shortType = resource.type.split('/').pop() ?? resource.type;
    let label = `${resource.name}\n${shortType}`;

    elements.push({
      group: 'nodes',
      data: {
        id: resource.id,
        resourceId: resource.id,
        label,
        borderColor: DIFF_COLORS[diffStatus],
        borderWidth: DIFF_BORDER_WIDTH[diffStatus],
        diffStatus,
      },
    });
  }

  // Add edges from connections (outbound only to avoid duplicates).
  for (const resource of resources) {
    for (const conn of resource.connections) {
      if (conn.direction === 'Outbound') {
        // Only add edge if target exists in the resource list.
        const targetExists = resources.some((r) => r.id === conn.id);
        if (targetExists) {
          elements.push({
            group: 'edges',
            data: {
              id: `${resource.id}-->${conn.id}`,
              source: resource.id,
              target: conn.id,
            },
          });
        }
      }
    }
  }

  return elements;
}

/**
 * Create a diff legend element showing color meanings.
 */
export function createDiffLegend(): HTMLElement {
  const legend = document.createElement('div');
  legend.className = 'radius-graph-legend';

  const items: Array<{ label: string; className: string }> = [
    { label: 'Added', className: 'radius-graph-legend-swatch--added' },
    { label: 'Modified', className: 'radius-graph-legend-swatch--modified' },
    { label: 'Removed', className: 'radius-graph-legend-swatch--removed' },
    { label: 'Unchanged', className: 'radius-graph-legend-swatch--unchanged' },
  ];

  for (const item of items) {
    const el = document.createElement('span');
    el.className = 'radius-graph-legend-item';
    el.innerHTML = `<span class="radius-graph-legend-swatch ${item.className}"></span>${item.label}`;
    legend.appendChild(el);
  }

  return legend;
}
