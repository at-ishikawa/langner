import { defineConfig } from "wxt";

// See https://wxt.dev/api/config.html
export default defineConfig({
  srcDir: ".",
  outDir: ".output",
  manifest: {
    name: "Langner Video Notebook Capture (PoC)",
    description:
      "Capture subtitles from streaming videos and save them as Langner story notebooks.",
    version: "0.0.2",
    permissions: ["activeTab", "scripting", "downloads"],
    host_permissions: [
      "*://www.youtube.com/*",
      "*://m.youtube.com/*",
      "*://*.netflix.com/*",
    ],
    action: {
      default_title: "Capture subtitles as notebook",
    },
  },
});
