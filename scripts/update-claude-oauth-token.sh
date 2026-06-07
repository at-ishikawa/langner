#!/usr/bin/env bash
# Update the CLAUDE_CODE_OAUTH_TOKEN GitHub Actions secret used by the
# Claude PR Review workflow (.github/workflows/claude-review.yml).
#
# Get a fresh token:
#   1. Run `claude setup-token` locally and complete the OAuth flow
#   2. Copy the long-lived token printed at the end
#   3. Run this script and paste the token when prompted
#
# Usage:
#   scripts/update-claude-oauth-token.sh [owner/repo]
#
# If owner/repo is omitted, the script uses the current git remote.

set -euo pipefail

REPO="${1:-}"

if [[ -z "$REPO" ]]; then
  REPO=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
fi

echo "Updating CLAUDE_CODE_OAUTH_TOKEN for $REPO"
echo
echo "Paste your Claude OAuth token (from \`claude setup-token\`), then press Enter:"
read -rs TOKEN
echo

if [[ -z "$TOKEN" ]]; then
  echo "Error: no token provided" >&2
  exit 1
fi

printf '%s' "$TOKEN" | gh secret set CLAUDE_CODE_OAUTH_TOKEN --repo "$REPO"
echo "Updated CLAUDE_CODE_OAUTH_TOKEN for $REPO"
