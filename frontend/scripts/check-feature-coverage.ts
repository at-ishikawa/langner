#!/usr/bin/env tsx
// Layer 2 coverage check: every route under src/app/**/page.tsx must be
// mentioned in at least one .feature file.
//
// Run: pnpm test:e2e:coverage
// Fails CI when a new page lands without a scenario that visits its URL.

import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const APP_DIR = join(process.cwd(), "src", "app");
const FEATURES_DIR = join(process.cwd(), "e2e", "features");
const STEPS_DIR = join(process.cwd(), "e2e", "steps");

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      walk(full, out);
    } else if (entry === "page.tsx") {
      out.push(full);
    }
  }
  return out;
}

function routeFromPagePath(pagePath: string): string {
  const rel = relative(APP_DIR, pagePath).replace(/page\.tsx$/, "");
  const segments = rel.split("/").filter(Boolean);
  if (segments.length === 0) return "/";
  return "/" + segments.join("/").replace(/\[([^\]]+)\]/g, "[$1]");
}

function readSources(): string {
  let combined = "";
  function recurse(dir: string, extPattern: RegExp) {
    for (const entry of readdirSync(dir)) {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) recurse(full, extPattern);
      else if (extPattern.test(entry)) combined += readFileSync(full, "utf8") + "\n";
    }
  }
  recurse(FEATURES_DIR, /\.feature$/);
  recurse(STEPS_DIR, /\.ts$/);
  return combined;
}

function routeMatchesText(route: string, text: string): boolean {
  // Dynamic segments match either an actual value used in code (a-zA-Z0-9_-)
  // or the literal placeholder form `[paramName]` used in comments and docs.
  const pattern = route
    .replace(/\[[^\]]+\]/g, "(\\[[^\\]]+\\]|[A-Za-z0-9_-]+)")
    .replace(/\//g, "\\/");
  return new RegExp(`(^|\\b|"|/)\\/?${pattern.replace(/^\\\//, "")}(\\b|"|$|/|\\?)`).test(text);
}

const pages = walk(APP_DIR);
const routes = pages.map(routeFromPagePath);
const featureText = readSources();

const uncovered = routes.filter((route) => !routeMatchesText(route, featureText));

if (uncovered.length === 0) {
  console.log(`✓ All ${routes.length} routes referenced by .feature scenarios.`);
  process.exit(0);
}

console.error("✗ Uncovered routes (no .feature scenario visits them):");
for (const r of uncovered) console.error(`  ${r}`);
console.error(`\nAdd a scenario that navigates to each uncovered route.`);
process.exit(1);
