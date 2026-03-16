/**
 * Format a next review date string (YYYY-MM-DD) into a human-readable form.
 * - Returns "tomorrow" if the date is tomorrow.
 * - Returns "in X days" if within 7 days.
 * - Returns a full human-readable date like "March 25, 2026" otherwise.
 */
export function formatReviewDate(dateStr: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(dateStr);
  if (!match) {
    return dateStr;
  }

  const year = parseInt(match[1], 10);
  const month = parseInt(match[2], 10) - 1;
  const day = parseInt(match[3], 10);
  const target = new Date(year, month, day);

  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const diffMs = target.getTime() - today.getTime();
  const diffDays = Math.round(diffMs / (1000 * 60 * 60 * 24));

  if (diffDays === 1) {
    return "tomorrow";
  }
  if (diffDays > 1 && diffDays <= 7) {
    return `in ${diffDays} days`;
  }

  return target.toLocaleDateString("en-US", {
    year: "numeric",
    month: "long",
    day: "numeric",
  });
}
