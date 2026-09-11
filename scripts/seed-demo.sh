#!/usr/bin/env bash
# Seeds a running School Nanny with demo kids, curricula, lessons, and tests.
#
# Usage:
#   ./scripts/run.sh &
#   ./scripts/seed-demo.sh
#
# Options via env:
#   BASE=http://127.0.0.1:8080   app URL (default shown)
#
# Kids are ensured by name (Lux/Emi/Max/Mei) and never duplicated. Curricula,
# lessons, and assessments are always added fresh, so re-running stacks more
# of those on top of what is already there.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="${BASE:-http://127.0.0.1:8080}"
CURRICULA="$ROOT/scripts/seed-data/test-curricula.yaml"

if ! curl -sf -o /dev/null "$BASE/"; then
    echo "School Nanny is not reachable at $BASE"
    echo "Start it first with ./scripts/run.sh, then run this again."
    exit 1
fi
if [ ! -f "$CURRICULA" ]; then
    echo "Missing curricula file: $CURRICULA"
    exit 1
fi

# --- helpers ---------------------------------------------------------------

post() {
    curl -sf -o /dev/null -X POST "$@"
}

# Prints "id name" lines for every child on Settings.
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

kid_id() {
    local name="$1" id
    id="$(list_kids | awk -v n="$name" '$2 == n { print $1; exit }')"
    if [ -z "$id" ]; then
        echo "could not find kid named $name" >&2
        exit 1
    fi
    printf '%s' "$id"
}

# Prints "id name" lines for every curriculum plan.
list_plans() {
    curl -sf "$BASE/curriculum" | awk '
        match($0, /href="\/curriculum\/([0-9]+)"/, m) { id=m[1]; next }
        /<strong>/ && id {
            match($0, /<strong>([^<]+)<\/strong>/, m)
            print id, m[1]
            id=""
        }
    '
}

plan_id() {
    local name="$1" id=""
    while read -r pid rest; do
        if [ "$rest" = "$name" ]; then
            id="$pid"
            break
        fi
    done < <(list_plans)
    if [ -z "$id" ]; then
        echo "could not find plan named $name" >&2
        exit 1
    fi
    printf '%s' "$id"
}

# Relative calendar day: day -1 = yesterday, 0 = today, 1 = tomorrow.
day() {
    date -d "$1 day" +%F 2>/dev/null || date -v"${1}d" +%F
}

TODAY="$(day 0)"
YESTERDAY="$(day -1)"
TWO_AGO="$(day -2)"
THREE_AGO="$(day -3)"
TOMORROW="$(day 1)"
IN_TWO="$(day 2)"

# Monday of the current week (ISO), for applying curricula.
WEEK_START="$(date -d "monday" +%F 2>/dev/null || date -v-monday +%F)"

# --- kids ------------------------------------------------------------------

echo "Kids"
"$ROOT/scripts/seed-kids.sh"

LUX="$(kid_id Lux)"
EMI="$(kid_id Emi)"
MAX="$(kid_id Max)"
MEI="$(kid_id Mei)"
echo "  ids: Lux=$LUX Emi=$EMI Max=$MAX Mei=$MEI"

# --- school year -----------------------------------------------------------

echo "School year"
YEAR_START="$(date -d '2026-08-01' +%F 2>/dev/null || echo 2026-08-01)"
YEAR_END="$(date -d '2027-05-31' +%F 2>/dev/null || echo 2027-05-31)"
post "$BASE/settings/years" \
    --data-urlencode "name=2026-2027" \
    --data-urlencode "starts_on=$YEAR_START" \
    --data-urlencode "ends_on=$YEAR_END" \
    --data "is_current=on"
echo "  2026-2027 (current)"

# --- curricula -------------------------------------------------------------

echo "Curricula"
IMPORT_HTML="$(mktemp)"
curl -sf -o "$IMPORT_HTML" -X POST "$BASE/curriculum/import" \
    -F "file=@$CURRICULA;type=application/x-yaml"
if grep -q 'form-error' "$IMPORT_HTML"; then
    echo "  import failed:" >&2
    grep -o 'form-error">[^<]*' "$IMPORT_HTML" | sed 's/form-error">/  /' >&2
    rm -f "$IMPORT_HTML"
    exit 1
fi
rm -f "$IMPORT_HTML"
echo "  imported plans from seed-data/test-curricula.yaml"

apply_plan() {
    local plan_name="$1" kid="$2"
    local pid
    pid="$(plan_id "$plan_name")"
    post "$BASE/curriculum/$pid/apply" \
        --data-urlencode "kid_id=$kid" \
        --data-urlencode "start=$WEEK_START" \
        --data-urlencode "weekday=1" \
        --data-urlencode "weekday=2" \
        --data-urlencode "weekday=3" \
        --data-urlencode "weekday=4" \
        --data-urlencode "weekday=5"
    echo "  applied \"$plan_name\" → kid $kid from $WEEK_START"
}

apply_plan "Lux Math" "$LUX"
apply_plan "Lux Language Arts" "$LUX"
apply_plan "Lux Japanese" "$LUX"
apply_plan "Emi Math" "$EMI"
apply_plan "Emi Language Arts" "$EMI"
apply_plan "Max Preschool" "$MAX"
apply_plan "Mei Play Day" "$MEI"

# --- one-off lessons (planned / done / overdue) ----------------------------
# Subjects: 1 math, 2 language-arts, 3 science, 6 japanese, 7 music-art, 8 other

echo "Extra lessons"
lesson() {
    # lesson kid subject date title minutes [status] [notes]
    local kid="$1" subject="$2" when="$3" title="$4" minutes="$5"
    local status="${6:-}" notes="${7:-}"
    local args=(
        --data-urlencode "kid_id=$kid"
        --data-urlencode "subject_id=$subject"
        --data-urlencode "scheduled_on=$when"
        --data-urlencode "title=$title"
        --data-urlencode "minutes=$minutes"
        --data-urlencode "notes=$notes"
        --data "back=/"
    )
    if [ "$status" = "done" ]; then
        args+=(--data "status=done")
    fi
    post "$BASE/lessons" "${args[@]}"
    echo "  $title ($when${status:+, $status})"
}

# Lux: some finished work, one overdue, something for tomorrow
lesson "$LUX" 1 "$TWO_AGO"   "Fractions with measuring cups" 30 done "Clicked after the second try."
lesson "$LUX" 3 "$YESTERDAY" "Volcano model, part 1" 45 done ""
lesson "$LUX" 5 "$THREE_AGO" "Map of the Oregon Trail" 25 "" ""   # overdue planned
lesson "$LUX" 2 "$TOMORROW"  "Essay draft: a local hero" 40 "" ""
lesson "$LUX" 7 "$IN_TWO"    "Watercolor: warm and cool" 45 "" ""

# Emi
lesson "$EMI" 1 "$YESTERDAY" "Counting bears to 15" 15 done ""
lesson "$EMI" 2 "$TODAY"     "Read-aloud: Frog and Toad" 20 "" ""
lesson "$EMI" 7 "$TOMORROW"  "Finger painting" 25 "" ""

# Max
lesson "$MAX" 8 "$YESTERDAY" "Block tower challenge" 15 done ""
lesson "$MAX" 7 "$TODAY"     "Playdough shapes" 20 "" ""

# Mei
lesson "$MEI" 8 "$TODAY"     "Bubble chase" 10 done ""
lesson "$MEI" 8 "$TOMORROW"  "Park stroll" 20 "" ""

# --- assessments -----------------------------------------------------------

echo "Assessments"
assess() {
    # assess kid subject date name score max [letter] [notes]
    local kid="$1" subject="$2" when="$3" name="$4"
    local score="${5:-}" max="${6:-}" letter="${7:-}" notes="${8:-}"
    local args=(
        --data-urlencode "kid_id=$kid"
        --data-urlencode "subject_id=$subject"
        --data-urlencode "given_on=$when"
        --data-urlencode "name=$name"
        --data-urlencode "notes=$notes"
        --data "back=/"
    )
    [ -n "$score" ]  && args+=(--data-urlencode "score=$score")
    [ -n "$max" ]    && args+=(--data-urlencode "max_score=$max")
    [ -n "$letter" ] && args+=(--data-urlencode "letter=$letter")
    post "$BASE/assessments" "${args[@]}"
    echo "  $name"
}

assess "$LUX" 1 "$TWO_AGO"   "Chapter 3: place value" 18 20 "" "Missed the last word problem."
assess "$LUX" 2 "$THREE_AGO" "Spelling list 4"        14 15 "" ""
assess "$LUX" 6 "$YESTERDAY" "Hiragana quiz"          42 50 "" ""
assess "$LUX" 3 "$YESTERDAY" "Science observation"    "" "" "A-" "Volcano write-up."
assess "$EMI" 1 "$YESTERDAY" "Counting check"          9 10 "" ""
assess "$EMI" 2 "$TWO_AGO"   "Letter sounds"           8 10 "" ""
assess "$MAX" 8 "$TWO_AGO"   "Color naming"            "" "" "M"  "Knew all but purple."

echo
echo "Done. Open $BASE — Today, Week, Curriculum, and Tests should all have data."
echo "Kids are never duplicated on re-run; curricula/lessons/tests are added again."
