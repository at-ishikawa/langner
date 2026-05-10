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

// Routes intentionally not exercised by any scenario yet. Each entry needs a
// reason so the gap is documented. Remove from this list once a feature covers it.
const INTENTIONALLY_UNCOVERED: Record<string, string> = {
  "/learn/[id]": "Story-notebook detail page; no story fixture in the test seed.",
};

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

const uncovered: string[] = [];
const skipped: { route: string; reason: string }[] = [];
for (const route of routes) {
  if (routeMatchesText(route, featureText)) continue;
  if (route in INTENTIONALLY_UNCOVERED) {
    skipped.push({ route, reason: INTENTIONALLY_UNCOVERED[route] });
  } else {
    uncovered.push(route);
  }
}

if (skipped.length > 0) {
  console.log(`Skipped (intentional, see INTENTIONALLY_UNCOVERED):`);
  for (const { route, reason } of skipped) console.log(`  ${route} — ${reason}`);
  console.log();
}

if (uncovered.length === 0) {
  console.log(
    `✓ ${routes.length - skipped.length}/${routes.length} routes referenced by .feature scenarios; ${skipped.length} intentionally skipped.`,
  );
  process.exit(0);
}

console.error("✗ Uncovered routes (no .feature scenario visits them):");
for (const r of uncovered) console.error(`  ${r}`);
console.error(
  `\nAdd a scenario that navigates to each uncovered route, or add it to INTENTIONALLY_UNCOVERED in this script with a reason.`,
);
process.exit(1);
