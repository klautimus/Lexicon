#!/bin/bash
# upgrade-library.sh — Bulk upgrade all tracks in the Lexicon library
# Uses the new bestaudio/opus pipeline to re-download higher quality versions
#
# Usage:
#   ./upgrade-library.sh                    # Upgrade all tracks
#   ./upgrade-library.sh --limit 10         # Upgrade first 10 tracks
#   ./upgrade-library.sh --track-id 42      # Upgrade specific track
#   ./upgrade-library.sh --dry-run          # List tracks without upgrading

set -euo pipefail

API_URL="${LEXICON_API_URL:-http://localhost:8787}"
DRY_RUN=false
LIMIT=0
TRACK_ID=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)   DRY_RUN=true; shift ;;
        --limit)     LIMIT="$2"; shift 2 ;;
        --track-id)  TRACK_ID="$2"; shift 2 ;;
        *)           echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Upgrade a specific track
upgrade_track() {
    local id="$1"
    echo "  Upgrading track $id..."
    local response
    response=$(curl -s -X POST "$API_URL/api/library/upgrade" \
        -H "Content-Type: application/json" \
        -d "{\"track_id\": $id}")
    local job_id
    job_id=$(echo "$response" | python3 -c "import sys,json; print(json.load(sys.stdin).get('job_id',''))" 2>/dev/null || echo "")
    if [[ -n "$job_id" ]]; then
        echo "    → Job $job_id queued"
    else
        echo "    → Error: $response"
    fi
}

# Single track mode
if [[ -n "$TRACK_ID" ]]; then
    echo "Upgrading track $TRACK_ID..."
    if [[ "$DRY_RUN" == "true" ]]; then
        curl -s "$API_URL/api/library/tracks?limit=1&offset=$((TRACK_ID-1))" | python3 -m json.tool 2>/dev/null || echo "Track $TRACK_ID"
    else
        upgrade_track "$TRACK_ID"
    fi
    exit 0
fi

# Bulk mode — get all track IDs
echo "Fetching track list from $API_URL..."
TRACKS_RESPONSE=$(curl -s -X POST "$API_URL/api/library/upgrade-all" \
    -H "Content-Type: application/json" \
    -d "{\"limit\": $LIMIT}")

TOTAL
TOTAL=$(echo "$TRACKS_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total_tracks',0))" 2>/dev/null || echo 0)
TRACK_IDS=$(echo "$TRACKS_RESPONSE" | python3 -c "import sys,json; ids=json.load(sys.stdin).get('track_ids',[]); print(' '.join(str(i) for i in ids))" 2>/dev/null || echo "")

if [[ "$TOTAL" -eq 0 ]]; then
    echo "No tracks found to upgrade."
    exit 0
fi

echo "Found $TOTAL tracks to upgrade."

if [[ "$DRY_RUN" == "true" ]]; then
    echo "Dry run — tracks that would be upgraded:"
    for id in $TRACK_IDS; do
        echo "  Track $id"
    done
    exit 0
fi

# Confirm
read -p "Proceed with upgrading $TOTAL tracks? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 0
fi

# Upgrade each track
SUCCESS=0
FAIL=0
for id in $TRACK_IDS; do
    if upgrade_track "$id"; then
        SUCCESS=$((SUCCESS + 1))
    else
        FAIL=$((FAIL + 1))
    fi
    # Small delay to avoid overwhelming the download queue
    sleep 0.5
done

echo ""
echo "Done: $SUCCESS upgraded, $FAIL failed out of $TOTAL total."
echo "Check download jobs at $API_URL/api/download/jobs for status."
