#!/usr/bin/env bash
# Adds the test kids to a running School Nanny instance.
# Re-running is safe: a name that is already there is left alone.
#
# Usage:
#   ./scripts/run.sh &          # start the app first
#   ./scripts/seed-kids.sh
#
# For kids plus curricula, lessons, and tests, use ./scripts/seed-demo.sh instead.
#
# Override the URL if needed:
#   BASE=http://127.0.0.1:8080 ./scripts/seed-kids.sh
#
# Remove duplicate Lux/Emi/Max/Mei rows (keeps the oldest of each name):
#   ./scripts/seed-kids.sh --dedupe
set -euo pipefail

BASE="${BASE:-http://127.0.0.1:8080}"

if ! curl -sf -o /dev/null "$BASE/"; then
    echo "School Nanny is not reachable at $BASE"
    echo "Start it first with ./scripts/run.sh, then run this again."
    exit 1
fi

# Prints "id name" lines for every child on Settings, oldest first.
list_kids() {
    curl -sf "$BASE/settings" | awk '
        /name="id"/ { match($0, /value="([0-9]+)"/, m); id=m[1] }
        /name="name"/ && id {
            match($0, /value="([^"]*)"/, m)
            print id, m[1]
            id=""
        }
    '
}

kid_exists() {
    local name="$1"
    list_kids | awk -v n="$name" '$2 == n { found=1; exit } END { exit !found }'
}

add_kid() {
    local name="$1" grade="$2" color="$3"
    if kid_exists "$name"; then
        echo "  $name already there — skipped"
        return
    fi
    curl -sf -o /dev/null -X POST "$BASE/settings/kids" \
        --data-urlencode "name=$name" \
        --data-urlencode "grade=$grade" \
        --data-urlencode "color=$color"
    echo "  added $name ($grade)"
}

# Keep the first (oldest) id for each of the seed names; delete the rest.
dedupe_kids() {
    local name ids keep rest
    echo "Deduping seed kids on $BASE ..."
    for name in Lux Emi Max Mei; do
        ids="$(list_kids | awk -v n="$name" '$2 == n { print $1 }')"
        if [ -z "$ids" ]; then
            echo "  $name — none found"
            continue
        fi
        keep="$(printf '%s\n' "$ids" | head -n1)"
        rest="$(printf '%s\n' "$ids" | tail -n +2)"
        if [ -z "$rest" ]; then
            echo "  $name — one copy (id $keep)"
            continue
        fi
        while read -r id; do
            [ -z "$id" ] && continue
            curl -sf -o /dev/null -X POST "$BASE/settings/kids/$id/delete"
            echo "  $name — removed duplicate id $id (kept $keep)"
        done <<< "$rest"
    done
    echo "Done."
}

if [ "${1:-}" = "--dedupe" ]; then
    dedupe_kids
    exit 0
fi

echo "Seeding kids on $BASE ..."
add_kid "Lux" "7" "#5b8def"
add_kid "Emi" "5" "#e0709a"
add_kid "Max" "3" "#3fae7f"
add_kid "Mei" "1" "#e0913f"
echo "Done. Open $BASE to check."
