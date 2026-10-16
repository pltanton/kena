# kena mistakes report

Date: 2026-10-17. Status: approved. Extends the kena and accounts specs.

## Goal

A logged-in learner's mistakes can be read by their agent over MCP and analysed
for weak spots; the agent can then build a targeted practice set. The site shows
the mistakes and suggests asking the agent.

## Error classification (`internal/greek`)

`Classify(given string, accepted []string) ErrorKind` for a wrong blank, comparing
normalised strings against each accepted answer and taking the closest:

- `empty` — nothing typed.
- `accent` — equal after stripping accents/diaeresis (NFD, drop marks).
- `final_sigma` — equal after mapping ς↔σ (and accents equal).
- `near_miss` — Levenshtein distance (runes) ≤ 2 after accent stripping.
- `other`.

## Store

`MistakesReport(ctx, progressID, setID *int64, since time.Time) (Report, error)`:
per task that has at least one wrong attempt in the window: set (id, slug,
title, topic, level), task (id, text, blanks with answers and hints,
explanation), `wrong_count`, `attempt_count`, `last_correct` (bool: latest
attempt overall was correct), and up to 5 most recent wrong attempts (given
answers, created_at). Plus per-set totals (tasks tried, accuracy). Ordered by
set, then wrong_count desc.

## MCP

`get_mistakes_report{set?, days?}` (default 30 days, max 365) — the caller's own
mistakes (their `progress_id`). Each wrong attempt is annotated per blank with
`kind` from Classify. Output also carries `summary`: counts by kind, and the
top 10 tasks by wrong_count still not fixed. Tool description tells the agent
to read the `analyze-mistakes` guide topic first.

## Guide

New topic `analyze-mistakes`: group errors by grammar point (set topic, hint
tags, lemma class) and by kind; separate spelling issues (accent, final ς) from
grammar issues (wrong form); explain the 2–4 main patterns to the user in plain
words with examples from their answers; then offer a practice set of 20–40 new
tasks targeting those patterns (new sentences, same rules as always), titled
"Practice: <pattern>"; never resubmit the
learner's original tasks verbatim. Add it to `Topics()` and mention in
Instructions only if space allows.

## Web

- `GET /mistakes` (login required → redirect to /login?next=/mistakes): per set,
  tasks with wrong attempts in the last 30 days: sentence with blanks shown as
  "given → expected", kind badge (accent / ς / near miss), wrong count, fixed ✓.
  Top: a box "Ask your agent: “Analyze my kena mistakes and make a practice set
  for my weak spots.”" with a copy button (inline JS, `navigator.clipboard`),
  and one line on how to connect an agent (link to /account).
- Links: header/home "Mistakes" when logged in; set done screen gets
  "Review mistakes" link when logged in.
- Anonymous users: home shows nothing new.

## Out of scope

Reports about other users (admin included), charts, scheduled reports.
