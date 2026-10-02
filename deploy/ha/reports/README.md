# Run reports

One directory per run, versioned in the repository. A run report is the evidence: without
it, nothing about RTO or RPO is proven (FR-050).

## Naming

```text
deploy/ha/reports/<YYYYMMDD-HHMMSS-mode>/
├── run.json   # timestamped data, stable format, comparable across runs
└── run.md     # readable table, generated from run.json, never hand-edited
```

The timestamp is UTC, and `<mode>` is the replication mode played by the run, with its
separator flattened to a dash: `postgres-streaming`, `sqlite-drbd`, `postgres-drbd`.

Example: `20270115-101200-sqlite-drbd/`.

The directory name is the `run_id` recorded inside `run.json`; the two never disagree.

## Format

`run.json` follows
[`specs/030-ha-opensvc/contracts/run-report.md`](../../../specs/030-ha-opensvc/contracts/run-report.md),
which is authoritative. In short, each report carries:

- `mode` — backend and replication;
- `versions` — Maintenant, OpenSVC, DRBD, PostgreSQL, kernel, bench commit. Mandatory: a
  report without them is not comparable, so it has no evidential value;
- `settings` — SQLite synchronous mode, heartbeat timeout, ready period, quorum, stonith,
  synchronous standby. Mandatory for the same reason;
- `scenarios` — one entry per scenario played, with its verdict, its repetitions and its
  measures (RTO, write classes, telemetry gap, induced alerts, agent reconnection,
  `nodes_written`, state inventory);
- `totals` — scenarios, pass, fail, not applicable.

## Rules that decide a run

- `writes.lost` is the only figure that decides the RPO. Any non-zero value fails the run,
  with no nuance.
- `nodes_written == 2` fails the run outright, whatever else happened.
- `inventory.outside_volume` non-empty fails the run, naming every file.
- The telemetry gap is reported separately and is never presented as data loss.
- `writes.unknown` is never counted as a loss, and never as a success either.
- A `not_applicable` scenario carries its reason, and counts neither as pass nor as fail.

## Comparing

`lab report --compare <run-a> <run-b>` renders the differences. The documented tolerance
covers timing measures only: verdicts must be identical.
