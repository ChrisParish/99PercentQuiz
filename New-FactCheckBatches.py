#!/usr/bin/env python3
"""Emits blind fact-check batches for a second model to answer.

Writes numbered prompt files containing questions with their answers removed.
Paste each file into a chat with a DIFFERENT model from the one that authored
the questions, then save the JSON reply to working/factcheck/reply-NN.json.

Merge the replies into one array and feed it back in:
    ./Test-Quiz.sh --path working/pool.json --fact-check-path working/factcheck/merged.json

Examples:
    ./New-FactCheckBatches.py
    ./New-FactCheckBatches.py --batch-size 25
"""
import argparse
import json
from pathlib import Path

INSTRUCTIONS = """You are fact-checking questions for a UK breakfast-radio quiz aimed at the general public.

Answer each question from your own knowledge. The quiz's own answers have been
withheld deliberately - do not try to guess what they were, just answer honestly.

For each question return:
  answer     - your answer, 4 words maximum, digits for numbers (45, not forty-five)
  difficulty - 1 to 5, judging what share of UK adults would answer correctly:
               1 = virtually all (99%+)   2 = most   3 = around half
               4 = a minority             5 = specialist knowledge
  ambiguous  - true if more than one answer is genuinely defensible
  reason     - short note, only when ambiguous is true, otherwise ""

Use UK English and UK conventions throughout.

Reply with ONLY a JSON array. No commentary before or after:

[{"id":"example-001","answer":"45","difficulty":1,"ambiguous":false,"reason":""}]"""


def main():
    script_dir = Path(__file__).resolve().parent
    parser = argparse.ArgumentParser(description="Emit blind fact-check batches for a second model to answer.")
    parser.add_argument("--pool-path", default=str(script_dir / "working" / "pool.json"))
    parser.add_argument("--out-dir", default=str(script_dir / "working" / "factcheck"))
    parser.add_argument("--batch-size", type=int, default=50)
    args = parser.parse_args()

    pool_path = Path(args.pool_path)
    out_dir = Path(args.out_dir)

    out_dir.mkdir(parents=True, exist_ok=True)
    for stale in out_dir.glob("batch-*.md"):
        stale.unlink()

    items = json.loads(pool_path.read_text(encoding="utf-8-sig"))

    batch_number = 0
    for start in range(0, len(items), args.batch_size):
        batch_number += 1
        batch = items[start:start + args.batch_size]

        lines = [f"# Fact-check batch {batch_number}", "", INSTRUCTIONS, "", "## Questions", ""]
        for item in batch:
            lines.append(f"{item.get('id')} | {item.get('question')}")

        path = out_dir / f"batch-{batch_number:02d}.md"
        path.write_text("\n".join(lines), encoding="utf-8")
        print(f"Wrote {path} ({len(batch)} questions)")

    print()
    print(f"{batch_number} batches covering {len(items)} questions.")
    print("Answer each batch with a different model, save replies to this folder, then merge and run:")
    print("  ./Test-Quiz.sh --path working/pool.json --fact-check-path working/factcheck/merged.json")


if __name__ == "__main__":
    main()
