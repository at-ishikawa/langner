// Content script that runs on video streaming pages.
// It injects a page-level script to intercept fetch/XHR subtitle responses,
// then listens for messages from the popup to return captured data.

export default defineContentScript({
  matches: ["*://www.youtube.com/*", "*://m.youtube.com/*", "*://*.netflix.com/*"],
  runAt: "document_start",
  main() {
    // Storage for captured subtitle data
    const capturedSubtitles: Array<{ url: string; content: string }> = [];

    // Listen for subtitle data from the injected page script
    window.addEventListener("message", (event) => {
      if (event.source !== window) return;
      if (event.data?.type !== "LANGNER_SUBTITLE_CAPTURED") return;

      capturedSubtitles.push({
        url: event.data.url,
        content: event.data.content,
      });
    });

    // Listen for messages from the popup requesting captured data
    chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
      if (message.type === "GET_CAPTURED_SUBTITLES") {
        sendResponse({ subtitles: capturedSubtitles });
        return true;
      }
      if (message.type === "GET_PAGE_METADATA") {
        sendResponse({
          title: extractTitle(),
          channel: extractChannel(),
          url: window.location.href,
        });
        return true;
      }
    });

    // Inject the interceptor script into the page context.
    // This runs in the page's JS context (not the content script sandbox),
    // so it can wrap the real fetch/XMLHttpRequest.
    injectPageScript();
  },
});

function extractTitle(): string {
  const meta = document.querySelector("ytd-watch-metadata #title h1");
  if (meta?.textContent?.trim()) return meta.textContent.trim();
  const pageTitle = document.querySelector("title")?.textContent?.trim() ?? "";
  return pageTitle.replace(/\s*-\s*YouTube\s*$/i, "").replace(/\s*\|\s*Netflix\s*$/i, "");
}

function extractChannel(): string {
  // YouTube
  const ytChannel = document.querySelector("ytd-video-owner-renderer #channel-name a");
  if (ytChannel?.textContent?.trim()) return ytChannel.textContent.trim();
  return "";
}

function injectPageScript() {
  const script = document.createElement("script");
  script.textContent = `(${pageInterceptor.toString()})();`;
  (document.head || document.documentElement).appendChild(script);
  script.remove();
}

// This function is stringified and injected into the page context.
// It cannot reference any imports or closures from the content script.
function pageInterceptor() {
  // URL patterns to match subtitle requests.
  // These are tested against the full request URL.
  function isSubtitleUrl(url: string): boolean {
    if (/youtube\.com\/api\/timedtext/i.test(url)) return true;
    if (/youtube\.com\/youtubei\/v1\/get_transcript/i.test(url)) return true;
    if (/nflxvideo\.net\/.*\.(?:ttml|xml|dfxp)/i.test(url)) return true;
    if (/\.vtt(?:\?|$)/i.test(url)) return true;
    if (/\.ttml(?:\?|$)/i.test(url)) return true;
    return false;
  }

  function postCapture(url: string, content: string) {
    window.postMessage(
      { type: "LANGNER_SUBTITLE_CAPTURED", url, content },
      "*",
    );
  }

  // Wrap fetch
  const originalFetch = window.fetch;
  window.fetch = async function (...args: Parameters<typeof fetch>) {
    const response = await originalFetch.apply(this, args);
    try {
      const url =
        typeof args[0] === "string"
          ? args[0]
          : args[0] instanceof Request
            ? args[0].url
            : String(args[0]);

      if (isSubtitleUrl(url)) {
        // Clone the response so the original consumer still gets the data
        const clone = response.clone();
        clone.text().then((text) => postCapture(url, text)).catch(() => {});
      }
    } catch {
      // Don't break the page if our interception fails
    }
    return response;
  };

  // Wrap XMLHttpRequest
  const originalOpen = XMLHttpRequest.prototype.open;
  const originalSend = XMLHttpRequest.prototype.send;
  const xhrUrlMap = new WeakMap<XMLHttpRequest, string>();

  XMLHttpRequest.prototype.open = function (
    method: string,
    url: string | URL,
    ...rest: unknown[]
  ) {
    xhrUrlMap.set(this, String(url));
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return (originalOpen as any).call(this, method, url, ...rest);
  };

  XMLHttpRequest.prototype.send = function (...args: unknown[]) {
    const xhr = this;
    const url = xhrUrlMap.get(xhr);
    if (url && isSubtitleUrl(url)) {
      xhr.addEventListener("load", function () {
        try {
          if (xhr.responseText) {
            postCapture(url, xhr.responseText);
          }
        } catch {
          // Ignore read errors
        }
      });
    }
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return (originalSend as any).apply(this, args);
  };
}
