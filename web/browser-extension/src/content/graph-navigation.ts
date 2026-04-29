// Graph navigation popup component.
// Appears when a user clicks a graph resource node, showing
// "Source code" and "App definition" links with security validation.

import type { ApplicationGraphResource, DiffStatus } from '../shared/graph-types.js';
import { isValidCodeReference, parseCodeReference, buildGitHubFileUrl } from './coderef-validator.js';

let activeOutsideClickHandler: ((event: MouseEvent) => void) | null = null;

export interface PopupContext {
  owner: string;
  repo: string;
  ref: string;
  appFile: string;
  pullNumber?: number;
}

/**
 * Create and show a navigation popup for a graph resource node.
 *
 * @param resource - The graph resource to show navigation for.
 * @param diffStatus - The diff status of the resource (affects link style).
 * @param context - GitHub context for URL construction.
 * @param container - The graph container element to append the popup to.
 * @param position - The screen position to show the popup at.
 */
export async function showGraphPopup(
  resource: ApplicationGraphResource,
  diffStatus: DiffStatus,
  context: PopupContext,
  container: HTMLElement,
  position: { x: number; y: number },
): Promise<void> {
  // Remove any existing popup first.
  closeGraphPopup(container);

  const popup = document.createElement('div');
  popup.className = 'radius-graph-popup';
  popup.id = 'radius-graph-popup';
  popup.style.left = `${position.x}px`;
  popup.style.top = `${position.y}px`;

  // Title: resource name.
  const title = document.createElement('div');
  title.className = 'radius-graph-popup-title';
  title.textContent = resource.name;
  popup.appendChild(title);

  // Type subtitle.
  const typeEl = document.createElement('div');
  typeEl.className = 'radius-graph-popup-type';
  typeEl.textContent = resource.type;
  popup.appendChild(typeEl);

  // Links container.
  const links = document.createElement('div');
  links.className = 'radius-graph-popup-links';

  // "Source code" link — only if codeReference is valid.
  if (resource.codeReference && isValidCodeReference(resource.codeReference)) {
    const { path, line } = parseCodeReference(resource.codeReference);
    const isDiffView = diffStatus === 'modified';
    const href = await buildGitHubFileUrl(
      { owner: context.owner, repo: context.repo, ref: context.ref, path, line, pullNumber: context.pullNumber },
      isDiffView,
    );

    const sourceLink = document.createElement('a');
    sourceLink.className = 'radius-graph-popup-link';
    sourceLink.href = href;
    sourceLink.target = '_blank';
    sourceLink.rel = 'noopener noreferrer';
    sourceLink.textContent = '📄 Source code';
    links.appendChild(sourceLink);
  }

  // "App definition" link — always shown, points to resource line in app.bicep.
  const appDefLine = resource.appDefinitionLine;
  const appDefHref = await buildGitHubFileUrl(
    {
      owner: context.owner,
      repo: context.repo,
      ref: context.ref,
      path: context.appFile,
      line: appDefLine,
      pullNumber: context.pullNumber,
    },
    diffStatus === 'modified',
  );

  const appDefLink = document.createElement('a');
  appDefLink.className = 'radius-graph-popup-link';
  appDefLink.href = appDefHref;
  appDefLink.target = '_blank';
  appDefLink.rel = 'noopener noreferrer';
  appDefLink.textContent = '📐 App definition';
  links.appendChild(appDefLink);

  popup.appendChild(links);

  // Close button.
  const closeBtn = document.createElement('button');
  closeBtn.className = 'radius-graph-popup-close';
  closeBtn.textContent = '×';
  closeBtn.addEventListener('click', (e) => {
    e.stopPropagation();
    closeGraphPopup(container);
  });
  popup.appendChild(closeBtn);

  // Stop mouse events from propagating to Cytoscape's container so that
  // link clicks are not intercepted.  Cytoscape's eventInContainer() walks
  // the DOM tree and will capture events from any child of its container,
  // calling preventDefault() on mousedown and firing a background-tap on
  // mouseup that removes the popup before the browser's click event fires.
  for (const eventType of ['mousedown', 'mouseup', 'touchstart', 'touchend', 'pointerdown', 'pointerup'] as const) {
    popup.addEventListener(eventType, (e) => e.stopPropagation());
  }

  container.appendChild(popup);

  // Close popup when clicking outside.
  const closeOnOutsideClick = (e: MouseEvent) => {
    if (!popup.contains(e.target as Node)) {
      closeGraphPopup(container);
    }
  };
  activeOutsideClickHandler = closeOnOutsideClick;

  // Defer listener to avoid immediate trigger.
  setTimeout(() => {
    if (activeOutsideClickHandler === closeOnOutsideClick) {
      document.addEventListener('click', closeOnOutsideClick);
    }
  }, 0);
}

/**
 * Remove any existing graph popup from the container.
 */
export function closeGraphPopup(container: HTMLElement): void {
  const existing = container.querySelector('#radius-graph-popup');
  if (existing) {
    existing.remove();
  }

  if (activeOutsideClickHandler) {
    document.removeEventListener('click', activeOutsideClickHandler);
    activeOutsideClickHandler = null;
  }
}
