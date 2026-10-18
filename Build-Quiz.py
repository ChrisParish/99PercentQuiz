#!/usr/bin/env python3
"""Merges the per-category question files into the working pool and the delivered file.

Reads working/categories/*.json, drops reserve questions, spreads every category
evenly across the running order so any 20-question day is varied, then writes:
  working/pool.json  - full records (id, category, acceptedAnswers)
  questions.json     - question and answer only, as the brief specifies

Example:
    ./Build-Quiz.py
"""
import argparse
import json
import random
from pathlib import Path


def main():
    script_dir = Path(__file__).resolve().parent
    parser = argparse.ArgumentParser(description="Merge per-category question files into the working pool.")
    parser.add_argument("--category-dir", default=str(script_dir / "working" / "categories"))
    parser.add_argument("--pool-path", default=str(script_dir / "working" / "pool.json"))
    parser.add_argument("--out-path", default=str(script_dir / "questions.json"))
    # Fixed seed keeps the running order reproducible between builds.
    parser.add_argument("--seed", type=int, default=99)
    args = parser.parse_args()

    category_dir = Path(args.category_dir)
    pool_path = Path(args.pool_path)
    out_path = Path(args.out_path)

    files = sorted(category_dir.glob("*.json"))
    if not files:
        raise SystemExit(f"No category files found in {category_dir}")

    all_items = []
    for file in files:
        loaded = json.loads(file.read_text(encoding="utf-8-sig"))
        all_items.extend(loaded)
        print(f"  {file.stem:<18} {len(loaded):>3} questions")

    main_items = [i for i in all_items if not i.get("reserve")]
    reserve_items = [i for i in all_items if i.get("reserve")]
    print()
    print(f"Main set: {len(main_items)}   Reserve: {len(reserve_items)}")

    # Spread each category evenly: position a question by how far through its own
    # category it sits, so a window of 20 draws proportionally from every category.
    rng = random.Random(args.seed)
    categories = sorted(set(i.get("category") for i in main_items))
    ordered = []
    for category in categories:
        group = [i for i in main_items if i.get("category") == category]
        shuffled = [item for _, item in sorted(((rng.random(), item) for item in group), key=lambda pair: pair[0])]
        n = len(shuffled)
        for i, item in enumerate(shuffled):
            ordered.append({"position": (i + 0.5) / n, "category": category, "item": item})

    ordered.sort(key=lambda x: (x["position"], x["category"]))
    final = [x["item"] for x in ordered]

    pool_path.parent.mkdir(parents=True, exist_ok=True)
    pool_path.write_text(json.dumps(final + reserve_items, indent=2), encoding="utf-8")

    delivered = [{"question": i.get("question"), "answer": i.get("answer")} for i in final]
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(delivered, indent=2), encoding="utf-8")

    print()
    print(f"Wrote {pool_path} ({len(final)} + {len(reserve_items)} reserve)")
    print(f"Wrote {out_path} ({len(delivered)} questions)")

    print()
    print("Category spread per broadcast day:")
    d = 0
    while d + 20 <= len(final):
        block = final[d:d + 20]
        cats = ", ".join(sorted(set(i.get("category") for i in block)))
        print(f"  Day {d // 20 + 1:>2}: {cats}")
        d += 20


if __name__ == "__main__":
    main()
