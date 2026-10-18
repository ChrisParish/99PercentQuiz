# The 99% Quiz — Authoring Rules

Single source of truth for both the human/LLM author and `Test-Quiz.ps1`.
If a rule changes here, change the corresponding check in the script.

## Target

- **800 questions** — 40 broadcast days × 20 questions.
- Answerable by **99% of UK adults**, and easy for anyone aged 10 or older in the UK.
  If you have to think, it's too hard.
- **Quick fire.** Read aloud in under 4 seconds, answered in one word or a short phrase.
- **No multiple choice.**
- **UK English throughout** — courgette, not zucchini.

## Category quotas

| Category            | Code       | Count |
| ------------------- | ---------- | ----- |
| Animals             | `animals`  | 100   |
| Simple science      | `science`  | 90    |
| General knowledge   | `general`  | 90    |
| UK geography        | `geography`| 80    |
| Food & drink        | `food`     | 80    |
| Sports              | `sports`   | 80    |
| Music (1990–present)| `music`    | 80    |
| True or False       | `truefalse`| 80    |
| Body & everyday life| `body`     | 80    |
| Maths               | `maths`    | 40    |
| **Total**           |            | **800** |

Maths is deliberately the smallest category at one per day. It is the only
category the validator can prove correct, but three a day felt unbalanced on air.
Retired maths questions stay in the pool flagged `reserve`.

`truefalse` is a *format*, not a topic — it draws subject matter from the others.
Phrase these statement-first — "Spiders are insects: true or false?" — so each
question has a distinct opening stem and passes rule 10.

A **reserve pool** of ~60 extra questions is authored on top, marked `"reserve": true`,
so fact-check rejects can be swapped without regenerating a whole category.

## Hard rules

1. **No superlative without a stated dimension.** Banned bare: `smallest`, `largest`,
   `biggest`, `fastest`, `slowest`, `best`, `worst`, `most`, `least`, `tallest`,
   `oldest`, `longest`, `strongest`, `highest`, `deepest`.
   "What is the smallest mammal?" differs by length or weight — bin it.
   Permitted only when the dimension is explicit *and* the answer is uncontested,
   e.g. "What is the largest planet in our solar system?".
2. **No "the only X that Y" claims.** These are almost always wrong or contested.
   Banned outright regardless of phrasing.
3. **Colour questions** only where the object is single-coloured by definition —
   a Royal Mail postbox, a London bus, a Ferrari badge. Never fruit, never sports
   balls, never "the sun".
4. **Answer** ≤ 4 words. Must not contain ` or `, `/`, `either`, `approximately`,
   `about`, `roughly`, `around`. One canonical string.
5. **Question** ≤ 12 words and ≤ 80 characters.
6. **Numbers as digits** in answers — `4`, not `four`. Consistent across the set.
7. **UK English.** Banned tokens: `zucchini`, `eggplant`, `cilantro`, `soccer`,
   `math`, `color`, `gray`, `sidewalk`, `elevator`, `apartment`, `candy`, `diaper`,
   `gasoline`, `truck`, `aluminum`, `cookie`, `pants`, `faucet`, `trash`, `garbage`,
   `flashlight`, `vacation`, `airplane`, `mom`, `license plate`, `dollars`, `cents`.
8. **No time-sensitive answers.** Banned tokens: `current`, `currently`, `today`,
   `now`, `latest`, `reigning`, `this year`, `last season`, `record holder`,
   `prime minister`, `number one`, `richest`, `recent`, `so far`, `to date`.
   Music must be historical fact — "Which band released Wonderwall?" is fine;
   "Who is number one?" is not.
9. **No drifting counts** — moons of a planet, world records, populations,
   number of countries, tallest building.
10. **Max 6 questions** may share the same 4-word opening stem. Kills template runs
    like 60 consecutive collective nouns.
11. **Max 8 questions** may share an identical answer. `True`/`False` are exempt,
    since the format guarantees repetition.
12. **ASCII only.** No smart quotes, em dashes or accented characters — studio
    playout systems mangle them.

## Maths rules

- Addition and subtraction: **two-digit** operands only.
- Multiplication: **single-digit** operands only.
- Written as `What is 18 + 27?` — spaces around the operator, `+`, `-`, `x`.
- `Test-Quiz.ps1` parses and evaluates every one of these. A wrong answer is a
  hard failure, so the entire maths category is verified without an LLM.

## Working pool schema

`working/pool.json` — **not** the deliverable.

```json
{
  "id": "maths-001",
  "category": "maths",
  "question": "What is 18 + 27?",
  "answer": "45",
  "acceptedAnswers": ["45", "forty five", "forty-five"],
  "reserve": false
}
```

## Delivered schema

`questions.json` — exactly as the brief specifies, nothing more.

```json
[
  { "question": "What is 2 + 2?", "answer": "4" }
]
```

`Build-Quiz.ps1` interleaves categories so that any contiguous block of 20 is
varied, then strips every field except `question` and `answer`.

## Fact-check thresholds

A blind second model answers each question without sight of ours, then rates it.
`Test-Quiz.ps1 -FactCheckPath` applies:

| Signal | Outcome |
| --- | --- |
| Blind answer not in `acceptedAnswers` | **Fail** — wrong, or our wording is too narrow |
| `difficulty` > 2 | **Fail** — breaches the 99% bar |
| `ambiguous` and blind answer **matched** | Warning — nuance for the presenter, not a trap |
| `ambiguous` and blind answer **differed** | Already failing on mismatch above |

Swapping a question invalidates its stored result. Re-verify the replacement
blind before merging again, or it will show up as a mismatch.
