// Layer 3 coverage: a Playwright reporter that records every URL visited during
// the run and diffs against the routes derived from src/app/**/page.tsx.
// Exits non-zero when any route isn't navigated to by any scenario.

import { readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import type {
  Reporter,
  TestCase,
  TestResult,
  TestStep,
  FullConfig,
  Suite,
} from "@playwright/test/reporter";

// Playwright's public TestStep interface omits `params`, but the in-process
// reporter receives it at runtime — that's how page.goto step urls reach us.
type StepWithParams = TestStep & { params?: { url?: string } };

const APP_DIR = join(process.cwd(), "src", "app");

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

  onTestEnd(test: TestCase, result: TestResult) {
    // Primary source: `visited-url` annotations recorded by the BeforeScenario
    // hook in steps/common.ts. Each framenavigated event (including the
    // click-driven router.push redirects) lands here. The annotations appear
    // on both TestCase (aggregated across retries) and TestResult (this run).
    const annotations = [...test.annotations, ...(result.annotations ?? [])];
    for (const annotation of annotations) {
      if (annotation.type === "visited-url" && annotation.description) {
        const route = urlToRoute(annotation.description, this.routes);
        if (route) this.visited.add(route);
      }
    }

    // Fallback: TestStep.params?.url for explicit page.goto in steps, plus
    // inner-step titles like `navigated to "<url>"`. Walks the full step
    // tree because the bdd Given/When/Then steps wrap the underlying
    // playwright actions.
    const TITLE_URL = /(?:Navigate(?:d)? to|page\.goto|navigated to)\s+"([^"]+)"/;

    const visit = (steps: TestStep[]) => {
      for (const step of steps ?? []) {
        const s = step as StepWithParams;
        const url = s.params?.url ?? step.title?.match(TITLE_URL)?.[1];
        if (typeof url === "string") {
          const route = urlToRoute(url, this.routes);
          if (route) this.visited.add(route);
        }
        if (step.steps && step.steps.length > 0) {
          visit(step.steps);
        }
      }
    };
    visit(result.steps);
  }

  async onEnd() {
    const uncovered = this.routes.filter((r) => !this.visited.has(r));

    console.log();
    console.log(`Route coverage (Layer 3, from actual navigations):`);
    console.log(`  visited:   ${this.visited.size}/${this.routes.length}`);
    console.log(`  uncovered: ${uncovered.length}`);
    if (uncovered.length > 0) {
      console.log(`\n  ✗ Not visited by any scenario:`);
      for (const r of uncovered) console.log(`      ${r}`);
      console.log(`\n    Add a scenario that navigates there.`);
      process.exitCode = 1;
    } else {
      console.log(`\n  ✓ Every route was navigated to.`);
    }
  }
}
