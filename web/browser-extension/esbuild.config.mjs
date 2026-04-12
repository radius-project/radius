// esbuild configuration for Manifest V3 content/background script bundling.
// Bundles TypeScript sources and npm dependencies (e.g., cytoscape) into
// dist/ for loading as a Chrome/Edge extension.

import * as esbuild from 'esbuild';
import { copyFileSync, mkdirSync, cpSync } from 'fs';
import { join } from 'path';

const isWatch = process.argv.includes('--watch');

/** @type {import('esbuild').BuildOptions} */
const commonOptions = {
  bundle: true,
  format: 'esm',
  target: 'es2022',
  sourcemap: true,
  outdir: 'dist',
  minify: !isWatch,
  logLevel: 'info',
};

/** Content script entry points */
const contentBuild = {
  ...commonOptions,
  entryPoints: ['src/content/inject.ts'],
  outdir: 'dist/content',
  // Content scripts run in page context — use iife for isolation
  format: 'iife',
};

/** Background service worker */
const backgroundBuild = {
  ...commonOptions,
  entryPoints: ['src/background/service-worker.ts'],
  outdir: 'dist/background',
  format: 'esm',
};

/** Popup page */
const popupBuild = {
  ...commonOptions,
  entryPoints: ['src/popup/popup.ts'],
  outdir: 'dist/popup',
  format: 'iife',
};

/** App create page */
const appCreateBuild = {
  ...commonOptions,
  entryPoints: ['src/app-create/app-create.ts'],
  outdir: 'dist/app-create',
  format: 'iife',
};

/** Copy static assets to dist/ */
function copyAssets() {
  mkdirSync('dist/popup', { recursive: true });
  mkdirSync('dist/content', { recursive: true });
  mkdirSync('dist/icons', { recursive: true });

  copyFileSync('manifest.json', 'dist/manifest.json');
  copyFileSync('src/popup/popup.html', 'dist/popup/popup.html');
  copyFileSync('src/popup/popup.css', 'dist/popup/popup.css');
  copyFileSync('src/content/styles.css', 'dist/content/styles.css');

  // Copy graph CSS if it exists
  try {
    mkdirSync('dist/content/styles', { recursive: true });
    copyFileSync('src/styles/graph.css', 'dist/content/styles/graph.css');
  } catch {
    // graph.css may not exist yet
  }

  cpSync('icons', 'dist/icons', { recursive: true });
}

async function build() {
  copyAssets();

  if (isWatch) {
    const contexts = await Promise.all([
      esbuild.context(contentBuild),
      esbuild.context(backgroundBuild),
      esbuild.context(popupBuild),
      esbuild.context(appCreateBuild),
    ]);

    await Promise.all(contexts.map((ctx) => ctx.watch()));
    console.log('Watching for changes...');
  } else {
    await Promise.all([
      esbuild.build(contentBuild),
      esbuild.build(backgroundBuild),
      esbuild.build(popupBuild),
      esbuild.build(appCreateBuild),
    ]);
    console.log('Build complete.');
  }
}

build().catch((err) => {
  console.error(err);
  process.exit(1);
});
