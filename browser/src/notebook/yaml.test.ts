import { describe, expect, it } from "vitest";
import { parse } from "yaml";
import { serializeStoryNotebooks } from "./yaml.js";
import type { StoryNotebook } from "../types.js";

function sampleNotebook(): StoryNotebook {
  return {
    event: "Sample Video Title",
    metadata: {
      series: "Sample Channel",
      season: 0,
      episode: 0,
    },
    date: new Date("2026-04-11T00:00:00.000Z"),
    scenes: [
      {
        scene: "Introduction",
        conversations: [
          { speaker: "", quote: "Hello and welcome to the channel." },
          { speaker: "", quote: "Today we are going to look at something new." },
        ],
        definitions: [],
      },
      {
        scene: "Main Topic",
        conversations: [
          { speaker: "", quote: "The main thing to understand here is context." },
        ],
        definitions: [],
      },
    ],
  };
}

describe("serializeStoryNotebooks", () => {
  it("wraps a single notebook in a YAML sequence", () => {
    const yaml = serializeStoryNotebooks([sampleNotebook()]);
    // Top level must be a sequence — the Langner reader expects a list.
    expect(yaml.trimStart().startsWith("-")).toBe(true);
  });

  it("round-trips: parsing the output produces equivalent data", () => {
    const notebook = sampleNotebook();
    const yaml = serializeStoryNotebooks([notebook]);
    const parsed = parse(yaml) as Array<Record<string, unknown>>;

    expect(parsed).toHaveLength(1);
    const first = parsed[0]!;
    expect(first.event).toBe("Sample Video Title");
    expect(first.metadata).toEqual({
      series: "Sample Channel",
      season: 0,
      episode: 0,
    });

    const scenes = first.scenes as Array<Record<string, unknown>>;
    expect(scenes).toHaveLength(2);
    expect(scenes[0]!.scene).toBe("Introduction");

    const conversations = scenes[0]!.conversations as Array<Record<string, unknown>>;
    expect(conversations).toHaveLength(2);
    expect(conversations[0]!.quote).toBe("Hello and welcome to the channel.");
  });

  it("preserves key order event → metadata → date → scenes", () => {
    const yaml = serializeStoryNotebooks([sampleNotebook()]);
    const eventIdx = yaml.indexOf("event:");
    const metadataIdx = yaml.indexOf("metadata:");
    const dateIdx = yaml.indexOf("date:");
    const scenesIdx = yaml.indexOf("scenes:");

    expect(eventIdx).toBeGreaterThanOrEqual(0);
    expect(eventIdx).toBeLessThan(metadataIdx);
    expect(metadataIdx).toBeLessThan(dateIdx);
    expect(dateIdx).toBeLessThan(scenesIdx);
  });

  it("omits the definitions field for scenes with no definitions", () => {
    const yaml = serializeStoryNotebooks([sampleNotebook()]);
    expect(yaml).not.toContain("definitions:");
  });

  it("includes definitions when a scene has any", () => {
    const notebook = sampleNotebook();
    notebook.scenes[0]!.definitions = [
      { expression: "break the ice", meaning: "to initiate conversation" },
    ];
    const yaml = serializeStoryNotebooks([notebook]);
    expect(yaml).toContain("definitions:");
    expect(yaml).toContain("break the ice");
    expect(yaml).toContain("to initiate conversation");
  });

  it("quotes strings that contain YAML-special characters safely", () => {
    const notebook = sampleNotebook();
    notebook.event = "Video: A 'weird' title — with, punctuation";
    notebook.scenes[0]!.conversations[0] = {
      speaker: "",
      quote: 'He said "hello" and then left.',
    };
    const yaml = serializeStoryNotebooks([notebook]);
    const parsed = parse(yaml) as Array<Record<string, unknown>>;
    expect(parsed[0]!.event).toBe("Video: A 'weird' title — with, punctuation");
    const scenes = parsed[0]!.scenes as Array<Record<string, unknown>>;
    const conversations = scenes[0]!.conversations as Array<Record<string, unknown>>;
    expect(conversations[0]!.quote).toBe('He said "hello" and then left.');
  });

  it("emits the date in ISO8601 timestamp form", () => {
    const yaml = serializeStoryNotebooks([sampleNotebook()]);
    // The Langner reader parses `date` as a YAML timestamp, so the output
    // should look like `2026-04-11T00:00:00Z` or similar — without quotes.
    expect(yaml).toMatch(/date:\s+2026-04-11/);
  });

  it("serializes multiple notebooks as a multi-entry sequence", () => {
    const n1 = sampleNotebook();
    n1.event = "First";
    const n2 = sampleNotebook();
    n2.event = "Second";
    const yaml = serializeStoryNotebooks([n1, n2]);
    const parsed = parse(yaml) as Array<Record<string, unknown>>;
    expect(parsed).toHaveLength(2);
    expect(parsed[0]!.event).toBe("First");
    expect(parsed[1]!.event).toBe("Second");
  });
});
