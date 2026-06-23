// URL patterns used to detect subtitle/caption network requests.
// Each pattern is tested against the full request URL.
// Add new services by adding regex patterns here.

export const SUBTITLE_URL_PATTERNS: RegExp[] = [
  // YouTube timedtext API (JSON and other formats)
  /youtube\.com\/api\/timedtext/i,
  /youtube\.com\/youtubei\/v1\/get_transcript/i,

  // Netflix TTML subtitles
  /nflxvideo\.net\/.*\.(?:ttml|xml|dfxp)/i,

  // Generic WebVTT files
  /\.vtt(?:\?|$)/i,

  // Generic TTML files
  /\.ttml(?:\?|$)/i,

  // Generic SRT files (less common over network, but possible)
  /\.srt(?:\?|$)/i,
];

export function isSubtitleUrl(url: string): boolean {
  return SUBTITLE_URL_PATTERNS.some((pattern) => pattern.test(url));
}
