// Layer 3 coverage: a Playwright reporter that records every URL visited during
// the run and diffs against the routes derived from src/app/**/page.tsx.
// Emits a summary at the end and exits non-zero when a route's coverage status
// disagrees with INTENTIONALLY_UNCOVERED in scripts/check-feature-coverage.ts.

import { readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import type {
  Reporter,
  TestCase,
  TestResult,
  FullConfig,
  Suite,
} from "@playwright/test/reporter";

const APP_DIR = join(process.cwd(), "src", "app");

// Routes only exercised by @wip features. Drop entries as those features have
// their selectors fixed and the @wip tag removed.
const INTENTIONALLY_UNCOVERED = new Set<string>([
  "/learn/[id]",
  "/notebooks/[id]",
  "/notebooks/etymology/[id]",
  "/notebooks/etymology/[id]/mindmap",
  "/quiz/complete",
  "/quiz/standard",
  "/quiz/reverse",
  "/quiz/freeform",
  "/quiz/etymology-standard",
  "/quiz/etymology-reverse",
  "/quiz/etymology-freeform",
]);

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) walk(full, out);
    else if (entry === "page.tsx") out.push(full);
  }
  return out;
}

function routeFromPagePath(pagePath: string): string {
  const rel = relative(APP_DIR, pagePath).replace(/page\.tsx$/, "");
  const segments = rel.split("/").filter(Boolean);
  return segments.length === 0 ? "/" : "/" + segments.join("/");
}

function urlToRoute(url: string, routes: string[]): string | undefined {
  let path: string;
  try {
    path = new URL(url).pathname;
  } catch {
    return undefined;
  }
  // Prefer the longest static-prefix match against known routes.
  const candidates = routes
    .map((route) => {
      const pattern =
        "^" +
        route.replace(/\[[^\]]+\]/g, "[^/]+").replace(/\//g, "\\/") +
        "/?$";
      return new RegExp(pattern).test(path) ? route : null;
    })
    .filter((r): r is string => r !== null);
  return candidates.sort((a, b) => b.length - a.length)[0];
}

export default class CoverageReporter implements Reporter {
  private routes: string[] = [];
  private visited = new Set<string>();

  onBegin(_config: FullConfig, _suite: Suite) {
    this.routes = walk(APP_DIR).map(routeFromPagePath);
  }

  onTestEnd(_test: TestCase, result: TestResult) {
    for (const step of result.steps ?? []) {
      // Playwright records navigations as steps with the URL in the title.
      const url = step.params?.url ?? step.title?.match(/^Navigate to "([^"]+)"/)?.[1];
      if (typeof url === "string") {
        const route = urlToRoute(url, this.routes);
        if (route) this.visited.add(route);
      }
    }
    for (const attachment of result.attachments) {
      if (attachment.name === "trace" || attachment.contentType?.includes("video")) continue;
    }
  }

  async onEnd() {
    const uncovered = this.routes.filter(
      (r) => !this.visited.has(r) && !INTENTIONALLY_UNCOVERED.has(r),
    );
    const skipped = this.routes.filter((r) => INTENTIONALLY_UNCOVERED.has(r));

    console.log();
    console.log(`Route coverage (Layer 3, from actual navigations):`);
    console.log(
      `  visited:        ${this.visited.size}/${this.routes.length}`,
    );
    console.log(`  intentional:    ${skipped.length}`);
    console.log(`  uncovered:      ${uncovered.length}`);
    if (uncovered.length > 0) {
      console.log(`\n  ✗ Not visited by any scenario:`);
      for (const r of uncovered) console.log(`      ${r}`);
      console.log(
        `\n    Add a scenario that navigates there, or document the gap in INTENTIONALLY_UNCOVERED.`,
      );
      process.exitCode = 1;
    } else {
      console.log(`\n  ✓ Every required route was navigated to.`);
    }
  }
}
