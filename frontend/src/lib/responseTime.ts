export function responseTimeSince(startTimeMs: number): bigint {
  return BigInt(Date.now() - startTimeMs);
}
