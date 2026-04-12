import { defineConfig } from "wxt";

// See https://wxt.dev/api/config.html
export default defineConfig({
  srcDir: ".",
  outDir: ".output",
  manifest: {
    name: "Langner YouTube Notebook Capture (PoC)",
    description:
      "Capture the transcript of a YouTube video you are watching and save it as a Langner story notebook.",
    version: "0.0.1",
    permissions: ["activeTab", "scripting", "downloads"],
    host_permissions: ["*://www.youtube.com/*", "*://m.youtube.com/*"],
    action: {
      default_title: "Capture this YouTube video",
    },
  },
});
