#!/usr/bin/env python3
"""Deterministic validator for The 99% Quiz question set.

Enforces the rules in rules.md. Runs against either the working pool
(working/pool.json, extra fields allowed) or the delivered file
(questions.json, --strict). Exits 1 if any hard failure is found.

Examples:
    ./Test-Quiz.py --path working/pool.json
    ./Test-Quiz.py --path questions.json --strict
    ./Test-Quiz.py --path working/pool.json --fact-check-path working/factcheck/merged.json
"""
import argparse
import json
import re
import sys
from collections import Counter
from pathlib import Path

failures = []
warnings = []


def add_failure(message):
    failures.append(message)


def add_warning(message):
    warnings.append(message)


def get_normalised_text(text):
    if text is None:
        return ""
    t = str(text).lower()
    t = re.sub(r"[^a-z0-9 ]", " ", t)
    t = re.sub(r"\s+", " ", t)
    return t.strip()


# --- Rule lists (mirror rules.md) -------------------------------------------

BARE_SUPERLATIVES = [
    "smallest", "largest", "biggest", "fastest", "slowest", "best", "worst",
    "most", "least", "tallest", "oldest", "longest", "strongest", "highest", "deepest",
]

# Rule 1 escape hatch: uncontested, dimension-explicit questions.
SUPERLATIVE_ALLOWLIST = [
    "what is the largest planet in our solar system",
    "what is the largest ocean on earth",
    "what is the largest country in the world by area",
    "what is the largest mammal on earth",
    "what is the longest river in the world",
    "what is the tallest mountain in the world",
    "what is the largest desert in the world",
    "what is the largest continent by area",
    "which is the largest of the great lakes",
    "what is the most common blood type in the uk",
]

AMERICANISMS = [
    "zucchini", "eggplant", "cilantro", "soccer", "sidewalk", "elevator",
    "apartment", "candy", "diaper", "gasoline", "aluminum", "cookie",
    "faucet", "trash", "garbage", "flashlight", "vacation", "airplane",
    "dollars", "cents", "color", "gray", "math ", "math?",
]

TIME_SENSITIVE = [
    "current", "currently", "today", "latest", "reigning", "this year",
    "last season", "record holder", "prime minister", "number one",
    "richest", "recent", "so far", "to date", "right now",
]

# Rule 3: only objects that are single-coloured by definition.
COLOUR_ALLOWLIST = [
    "postbox", "london bus", "london taxi", "black cab", "uk passport",
    "snooker cue ball", "referee", "yellow card", "red card", "zebra crossing",
    "polar bear", "flamingo", "royal mail",
]

ANSWER_HEDGES = [" or ", "/", "either", "approximately", "about ", "roughly", "around "]

SPELLED_OUT_NUMBER = re.compile(
    r"^(one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve)$"
)
MULTIPLE_CHOICE = re.compile(r"\b[abcd]\)", re.IGNORECASE)
COLOUR_QUESTION = re.compile(r"what colou?r (is|are)", re.IGNORECASE)
ARITHMETIC = re.compile(r"^\s*What is\s+(\d+)\s*([+\-x*])\s*(\d+)\s*\?\s*$", re.IGNORECASE)

CATEGORY_QUOTAS = {
    "maths": 40, "animals": 100, "science": 90, "general": 90, "geography": 80,
    "food": 80, "sports": 80, "music": 80, "truefalse": 80, "body": 80,
}


def word_count(text):
    return len(text.split())


def validate(items, strict, expected_count):
    print(f"Validating {len(items)} questions")

    if strict and len(items) != expected_count:
        add_failure(f"Count is {len(items)}, expected {expected_count}.")

    for index, item in enumerate(items, start=1):
        label = f"[{index}]"
        if not isinstance(item, dict):
            add_failure(f"{label} is not an object.")
            continue
        props = list(item.keys())

        if "question" not in props:
            add_failure(f"{label} missing 'question'.")
            continue
        if "answer" not in props:
            add_failure(f"{label} missing 'answer'.")
            continue

        if strict:
            extra = [p for p in props if p not in ("question", "answer")]
            if extra:
                add_failure(f"{label} has disallowed fields: {', '.join(extra)}")

        q = str(item["question"])
        a = str(item["answer"])

        if not q.strip():
            add_failure(f"{label} question is empty.")
            continue
        if not a.strip():
            add_failure(f"{label} answer is empty.")
            continue

        q_lower = q.lower()
        a_lower = a.lower()
        q_norm = get_normalised_text(q)

        # Rule 12: ASCII only.
        for name, value in (("question", q), ("answer", a)):
            if re.search(r"[^\x00-\x7F]", value):
                add_failure(f"{label} non-ASCII in {name}: '{value}'")

        # Rule 5: question length.
        words = word_count(q)
        if words > 12:
            add_failure(f"{label} question is {words} words (max 12): '{q}'")
        if len(q) > 80:
            add_failure(f"{label} question is {len(q)} chars (max 80): '{q}'")
        if not q.endswith("?"):
            add_failure(f"{label} question does not end with '?': '{q}'")

        # Rule 4: answer shape.
        a_words = word_count(a)
        if a_words > 4:
            add_failure(f"{label} answer is {a_words} words (max 4): '{a}'")
        for hedge in ANSWER_HEDGES:
            if hedge in a_lower:
                add_failure(f"{label} answer contains hedge '{hedge.strip()}': '{a}'")

        # Rule 1: bare superlatives.
        if q_norm not in SUPERLATIVE_ALLOWLIST:
            for sup in BARE_SUPERLATIVES:
                if re.search(rf"\b{sup}\b", q_norm):
                    add_failure(f"{label} ambiguous superlative '{sup}': '{q}'")

        # Rule 2: "only X that Y" claims.
        if re.search(r"\bonly\b", q_lower):
            add_failure(f"{label} uses an 'only ...' claim: '{q}'")

        # Rule 3: colour questions.
        if COLOUR_QUESTION.search(q_lower):
            allowed = any(subject in q_lower for subject in COLOUR_ALLOWLIST)
            if not allowed:
                add_failure(f"{label} colour question on a multi-coloured subject: '{q}'")

        # Rule 7: UK English.
        for us in AMERICANISMS:
            if us in q_lower or us in a_lower:
                add_failure(f"{label} Americanism '{us.strip()}': '{q}' / '{a}'")

        # Rule 8: time-sensitive.
        for ts in TIME_SENSITIVE:
            if ts in q_lower or ts in a_lower:
                add_failure(f"{label} time-sensitive term '{ts}': '{q}' / '{a}'")

        # Rule 6: spelled-out numbers in answers.
        if SPELLED_OUT_NUMBER.match(a_lower):
            add_failure(f"{label} answer should use digits, not words: '{a}'")

        # No multiple choice.
        if MULTIPLE_CHOICE.search(q) or "which of the following" in q_lower:
            add_failure(f"{label} looks like multiple choice: '{q}'")

    # --- Arithmetic self-check ---------------------------------------------

    maths_checked = 0
    for item in items:
        if not isinstance(item, dict):
            continue
        q = str(item.get("question", ""))
        m = ARITHMETIC.match(q)
        if not m:
            continue

        left = int(m.group(1))
        op = m.group(2).lower()
        right = int(m.group(3))

        if op == "+":
            expected = left + right
        elif op == "-":
            expected = left - right
        else:
            expected = left * right

        maths_checked += 1
        actual = str(item.get("answer", "")).strip()
        if actual != str(expected):
            add_failure(f"ARITHMETIC '{q}' answer is '{actual}', should be '{expected}'.")

        if op in ("x", "*"):
            if left > 9 or right > 9:
                add_failure(f"'{q}' multiplication must use single digits.")
        else:
            if left > 99 or right > 99:
                add_failure(f"'{q}' must use two-digit operands.")
            if expected < 0:
                add_failure(f"'{q}' gives a negative answer.")

    # --- Duplicates ----------------------------------------------------------

    seen = {}
    for item in items:
        if not isinstance(item, dict):
            continue
        norm = get_normalised_text(item.get("question"))
        if norm in seen:
            add_failure(f"Duplicate question: '{item.get('question')}'")
        else:
            seen[norm] = True

    # Near-duplicates (Jaccard over word sets).
    token_sets = []
    for item in items:
        words = get_normalised_text(item.get("question") if isinstance(item, dict) else None).split(" ")
        token_sets.append(set(w for w in words if w))
    for i in range(len(token_sets)):
        for j in range(i + 1, len(token_sets)):
            a_set, b_set = token_sets[i], token_sets[j]
            if not a_set or not b_set:
                continue
            inter = len(a_set & b_set)
            union = len(a_set) + len(b_set) - inter
            if union > 0 and (inter / union) > 0.8:
                add_warning(
                    f"Near-duplicate: '{items[i].get('question')}' ~ '{items[j].get('question')}'"
                )

    # --- Rule 10: stem runs ---------------------------------------------------

    stems = Counter()
    for item in items:
        words = [w for w in get_normalised_text(item.get("question") if isinstance(item, dict) else None).split(" ") if w]
        if len(words) < 4:
            continue
        stems[" ".join(words[:4])] += 1
    for stem, count in stems.items():
        if count > 6:
            add_failure(f"Template run: {count} questions start '{stem}' (max 6).")

    # --- Rule 11: answer repetition --------------------------------------------

    answer_counts = Counter()
    for item in items:
        key = get_normalised_text(item.get("answer") if isinstance(item, dict) else None)
        answer_counts[key] += 1
    for key, count in answer_counts.items():
        # 'true'/'false' are inherent to the True or False format, so exempt.
        if key in ("true", "false"):
            continue
        if count > 8:
            add_failure(f"Answer '{key}' used {count} times (max 8).")

    # --- Category quotas and spread (pool mode) --------------------------------

    has_category = bool(items) and isinstance(items[0], dict) and "category" in items[0]
    if has_category and not strict:
        main = [i for i in items if not i.get("reserve")]
        by_cat = Counter(i.get("category") for i in main)
        for cat, quota in CATEGORY_QUOTAS.items():
            found = by_cat.get(cat, 0)
            if found != quota:
                add_warning(f"Category '{cat}' has {found} questions, quota is {quota}.")
        for cat in by_cat:
            if cat not in CATEGORY_QUOTAS:
                add_failure(f"Unknown category '{cat}'.")

    # Any contiguous block of 20 should span at least 6 categories.
    if has_category and len(items) >= 20:
        broadcast = [i for i in items if not i.get("reserve")]
        i = 0
        while i + 20 <= len(broadcast):
            block = broadcast[i:i + 20]
            distinct = len(set(b.get("category") for b in block))
            if distinct < 6:
                add_warning(f"Day {i // 20 + 1} spans only {distinct} categories (min 6).")
            i += 20

    # --- Read-aloud timing -----------------------------------------------------

    total_words = 0
    for item in items:
        if not isinstance(item, dict):
            continue
        total_words += word_count(str(item.get("question", "")))
        total_words += word_count(str(item.get("answer", "")))
    if items:
        seconds_per_day = (total_words / len(items)) * 20 / 2.5
        print(f"Estimated read time: {seconds_per_day:,.0f}s per 20-question day (words only, excl. banter).")

    return maths_checked


def merge_fact_check(items, fact_check_path):
    if not fact_check_path.exists():
        raise SystemExit(f"Fact-check file not found: {fact_check_path}")
    fc = json.loads(fact_check_path.read_text(encoding="utf-8-sig"))

    by_id = {str(item["id"]): item for item in items if isinstance(item, dict) and item.get("id")}
    fc_ids = {str(result.get("id")) for result in fc}

    for result in fc:
        item = by_id.get(str(result.get("id")))
        if not item:
            add_failure(f"FACTCHECK unknown id '{result.get('id')}'.")
            continue

        accepted = list(item.get("acceptedAnswers") or [])
        accepted.append(str(item.get("answer")))
        accepted_norm = [get_normalised_text(x) for x in accepted]

        blind = get_normalised_text(result.get("answer"))
        matched = blind in accepted_norm
        if not matched:
            add_failure(
                f"FACTCHECK mismatch on '{item.get('question')}': "
                f"ours '{item.get('answer')}', blind '{result.get('answer')}'."
            )
        if int(result.get("difficulty", 0)) > 2:
            add_failure(f"FACTCHECK too hard (difficulty {result.get('difficulty')}): '{item.get('question')}'")
        if result.get("ambiguous") is True:
            # Reviewer landed on an answer we accept, so the noted alternative is a
            # nuance for the presenter rather than a trap. Genuine traps fail above.
            if matched:
                add_warning(f"FACTCHECK nuance on '{item.get('question')}' - {result.get('reason')}")
            else:
                add_failure(f"FACTCHECK ambiguous: '{item.get('question')}' - {result.get('reason')}")

    missing = [i for i in items if isinstance(i, dict) and i.get("id") and str(i["id"]) not in fc_ids]
    if missing:
        add_warning(f"{len(missing)} questions have no fact-check result.")


def main():
    parser = argparse.ArgumentParser(description="Deterministic validator for The 99% Quiz question set.")
    parser.add_argument("--path", required=True, help="Path to the JSON file to validate.")
    parser.add_argument("--strict", action="store_true", help="Delivered-file mode: exactly N items, question/answer only.")
    parser.add_argument("--fact-check-path", help="Optional blind second-model results to fold in.")
    parser.add_argument("--expected-count", type=int, default=800)
    args = parser.parse_args()

    path = Path(args.path)
    if not path.exists():
        raise SystemExit(f"File not found: {path}")

    raw = path.read_text(encoding="utf-8-sig")
    try:
        items = json.loads(raw)
    except json.JSONDecodeError as e:
        raise SystemExit(f"Invalid JSON in {path}: {e}")
    if not isinstance(items, list):
        raise SystemExit(f"Root of {path} must be a JSON array.")

    maths_checked = validate(items, args.strict, args.expected_count)

    if args.fact_check_path:
        merge_fact_check(items, Path(args.fact_check_path))

    print()
    print(f"Arithmetic questions verified: {maths_checked}")

    if warnings:
        print()
        print(f"WARNINGS ({len(warnings)})")
        for w in warnings:
            print(f"  - {w}")

    if failures:
        print()
        print(f"FAILURES ({len(failures)})")
        for f in failures:
            print(f"  - {f}")
        print()
        print("FAILED")
        sys.exit(1)

    print()
    print("PASSED")
    sys.exit(0)


if __name__ == "__main__":
    main()
