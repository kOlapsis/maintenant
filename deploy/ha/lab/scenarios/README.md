# Bench scenarios

`lab chaos <n>` injects a failure, `lab heal <n>` repairs it. Every scenario is replayable at
will and leaves no residual state: `lab up`, `lab down`, `lab up` starts from an identical
state, and no run depends on the previous one (FR-047).

This file is the reference list. It exists so that someone outside the development team can
rebuild the bench and replay the campaign without asking a single question (SC-008). Each
scenario also has its own file, `NN-<slug>.yml`, carrying the same content in machine-readable
form; when the two disagree, the YAML file is authoritative for execution and this table is
authoritative for intent.

## Reading the table

- **Injection** is what `lab chaos <n>` does. It comes from the bench plan.
- **Repair** is what `lab heal <n>` does, and it must bring the bench back to nominal state
  before the next scenario runs. `lab run-all` repairs after every scenario and verifies the
  repair before moving on.
- **Expected** is defined per mode. The four modes are `ip-only`, `postgres/streaming`,
  `sqlite/drbd` and `postgres/drbd` (the last one only if it is played). A scenario with no
  object in a mode is reported *not applicable*, with its reason. It is never reported as
  passed, and its number is never reused.
- **Measures** lists the figures that decide the verdict. Every run collects the full
  measurement set anyway (see `contracts/run-report.md`); the column names what matters here.

Two probes run continuously on the measurement machine between `lab up` and `lab down`: the
availability probe (one request on the service address every 100 ms) and the write generator
(numbered writes, `ack` / `refused` / `unknown`, full re-read after recovery). No scenario
ever restarts them.

## Numbering rule (FR-047)

**Numbers 1 to 12 belong to the bench plan, permanently.** They are reserved even in a mode
where a scenario has no object: in `sqlite/drbd`, scenario 10 is reported *not applicable*,
and the number 10 is not handed to anything else.

**Mode-specific scenarios (FR-048) take free numbers, starting at 13.** The allocation below
is fixed here so that later phases cannot collide:

| # | Scenario | Modes |
|---|---|---|
| 13 | Degraded storage: process alive, database unreachable | all |
| 14 | Binary update | all |
| 15 | Standby absent while writes continue | `postgres/streaming` |
| 16 | Former primary returns and is rebuilt as standby | `postgres/streaming` |
| 17 | Write-ahead log retention exhausted | `postgres/streaming` |
| 18 | Forced promotion | `postgres/streaming` |
| 19 | Full giveback to the preferred node | `postgres/streaming` |
| 20 | SQLite journal recovery after crash | `sqlite/drbd` |
| 21 | Volume resynchronisation when the node returns | `sqlite/drbd` |
| 22 | Storage split-brain and its manual resolution | `sqlite/drbd` |

One provisional name used while planning, `11-degraded-network`, is dropped: number 11 is the
plan's degraded WAN scenario, which already covers it. The two others, `13-degraded-storage`
and `14-binary-update`, keep their numbers, which are free.

---

## The twelve scenarios of the plan

### 1. Clean switchover

| | |
|---|---|
| **Injection** | `om <service> switch` — a requested, orderly move to the other node. |
| **Repair** | Bring the service back to the preferred node with `lab giveback`, refused while that node's copy is not up to date. |
| **Expected, all modes** | The reference case, and the floor RTO every other scenario is compared against. Resources stop in reverse order and leave the data clean: journal flushed, volume demoted or database stopped properly. No acknowledged write is lost. Exactly one node holds the service throughout. |
| **Measures** | `rto_ms`, `writes`, `telemetry_gap_s`, `alerts_induced`, `agents`. |

### 2. Abrupt loss of the active node

| | |
|---|---|
| **Injection** | `virsh destroy site-a` from the host — the active node disappears, with no shutdown and no warning. |
| **Repair** | Restart the VM. It rejoins the cluster, its copy resynchronises from the survivor, and the service stays where it is: no automatic giveback. |
| **Expected, all modes** | Automatic failover with no human action. The service answers again on the same address in under 60 s, never more than 90 s across all runs (SC-001). `writes.lost == 0` over at least 20 repetitions (SC-002). Every enrolled agent reconnects within 90 s with the same identity, with no re-enrolment and no alert caused by the failover alone (SC-007). |
| **Expected, `postgres/streaming`** | The standby is promoted, exactly one primary is established before the application starts, and the returning node is rebuilt as a standby — never restarted as a primary (SC-011). |
| **Expected, `sqlite/drbd`** | The volume is promoted on the survivor, the file system is mounted, the SQLite journal is recovered, then the application starts. |
| **Measures** | `rto_ms`, `writes`, `telemetry_gap_s`, `telemetry_samples_lost`, `alerts_induced`, `agents`, `nodes_written`, `inventory`. |

This is the scenario the whole bench exists for. It is played with repetitions, not once.

### 3. Kernel panic

| | |
|---|---|
| **Injection** | `echo c > /proc/sysrq-trigger` on the active node. |
| **Repair** | Reboot the node and let it rejoin; the copy resynchronises from the survivor. |
| **Expected, all modes** | Same outcome as scenario 2, reached without any orderly shutdown: nothing is flushed, nothing is unmounted. Together with scenario 2 this is the RPO campaign — at least 20 repetitions per mode, zero missing acknowledged write (SC-002). |
| **Measures** | `rto_ms`, `writes`, `telemetry_gap_s`, `agents`, `nodes_written`, `inventory`. |

### 4. Freeze of the cluster agent, without a crash

| | |
|---|---|
| **Injection** | `kill -STOP` on the OpenSVC agent of the active node. The daemon stops answering but the node, the application and the replicated volume are all still alive. |
| **Repair** | `kill -CONT` on the agent, or, when the node was fenced, restart it and let it rejoin. |
| **Expected, all modes** | Heartbeat expiry detects the freeze. Detection alone is not recovery: a frozen daemon does not kill itself, and the survivor cannot take the data while the peer still holds it. **The frozen node is fenced before anything starts anywhere else.** If the fencing fails, nothing starts elsewhere and the operator is alerted — a failed fencing blocks, it never lets through (FR-024). |
| **Measures** | `rto_ms`, `nodes_written`, `alerts_induced`, fencing outcome. |

### 5. Inter-site partition

| | |
|---|---|
| **Injection** | `iptables -A` on the interconnect network, cutting the two sites from each other while both still reach the arbitrator. |
| **Repair** | Flush the rule; the cluster reconverges. |
| **Expected, `postgres/streaming`, `sqlite/drbd`, `postgres/drbd`** | The arbitrator decides. Exactly one side keeps or takes the service; the other stands down. After repair, verification shows a single side wrote: `nodes_written` is never 2, and no data diverged (SC-004). |
| **Expected, `ip-only`** | The arbitrator decides. Exactly one side keeps or takes the service; the other stands down. `nodes_written` is never 2. Data is local to each node in this mode, so writes are not compared. |
| **Measures** | `nodes_written`, `writes`, `rto_ms`, post-repair divergence check. |

### 6. Loss of the service address

| | |
|---|---|
| **Injection** | `ip addr del` on the active node, removing the service address from under the running service. |
| **Repair** | None beyond the injection: the resource monitor is what is being tested. Verify the address is back and reachable. |
| **Expected, all modes** | The resource monitor brings the address back on the same node. No failover. The outage lasts as long as one monitoring period, and no more. |
| **Measures** | `rto_ms`, `alerts_induced`. |

### 7. Application crash

| | |
|---|---|
| **Injection** | `kill -9` on the Maintenant process. |
| **Repair** | None beyond the injection: the bounded local restart is what is being tested. |
| **Expected, all modes** | Bounded local restart on the same node, **before** any failover. Service restored in under 30 s, with no failover (SC-005). The number of local restarts is an explicit setting, recorded in the run report. |
| **Measures** | `rto_ms`, `alerts_induced`, local restart count. |

### 8. Application freeze

| | |
|---|---|
| **Injection** | `kill -STOP` on the Maintenant process. |
| **Repair** | `kill -CONT`, or restart the node when it was fenced. |
| **Expected, all modes** | The process is alive and the service is dead. A PID check passes here and is therefore worthless: the application check must be an HTTP one, reading the body of `/api/v1/health`. Detection, then recovery, in under 120 s (SC-005). |
| **Measures** | `rto_ms`, detection delay, `alerts_induced`. |

This is the failure mode most HA setups miss. It is one of the three that decide whether the design is sound.

### 9. Full volume

| | |
|---|---|
| **Injection** | `fallocate` on the service volume until it is full. |
| **Repair** | Remove the allocated file and verify the database and the journal recovered. |
| **Expected, `postgres/streaming`, `sqlite/drbd`, `postgres/drbd`** | Controlled degradation, no corruption. The journal — SQLite's or PostgreSQL's — can no longer grow, and the product must degrade rather than corrupt. No failover loop toward a node whose volume is just as full: moving the service does not create space. |
| **Expected, `ip-only`** | *Not applicable* — no service volume in this mode. |
| **Measures** | `writes` (refusals are expected here and are not losses), `inventory`, absence of a failover loop. |

### 10. Loss of the PostgreSQL primary

| | |
|---|---|
| **Injection** | `kill -9` on the PostgreSQL primary. A variant makes the local restart impossible, to force the promotion path. |
| **Repair** | Let the local restart happen; in the variant, rebuild the former primary as a standby. |
| **Expected, `ip-only`** | *Not applicable* — no PostgreSQL in this mode. |
| **Expected, `postgres/streaming`** | Bounded local restart of the primary — crash recovery, standby still attached. The standby is promoted **only if** the restart fails: promoting on a mere process death would contradict "local restart before failover" and would force a rebuild for nothing. In the variant, promotion happens, exactly one primary is established, and the former primary is locked out of write service until it is rebuilt. |
| **Expected, `sqlite/drbd`** | *Not applicable* — there is no external database in this mode. |
| **Expected, `postgres/drbd`** | Local restart, if this mode is played. |
| **Measures** | `rto_ms`, `writes`, local restart count, primary count after recovery. |

### 11. Degraded WAN

| | |
|---|---|
| **Injection** | `tc netem delay 80ms loss 3%` on the interconnect network — latency and packet loss, without a cut. |
| **Repair** | `tc qdisc del` on the interconnect. |
| **Expected, all modes** | No failover, and above all no repeated failover. A degraded link is not a dead node; the timeouts must be wide enough to ride it out and the service must not flap. |
| **Measures** | Failover count (expected: zero), `rto_ms` if any failover happened, `alerts_induced`. |

### 12. Forced split-brain

| | |
|---|---|
| **Injection** | Partition of the interconnect **and** the arbitrator made unreachable. Played in two variants: unreachable from one side, then unreachable from both. |
| **Repair** | Restore the interconnect and the arbitrator, then run the post-mortem check before anything else. |
| **Expected, all modes** | With the arbitrator reachable from one side only: that side keeps or takes the service, and the isolated side crashes without having started anything (`split_action`). With the arbitrator unreachable from both sides: both crash. That is a total outage, and it is the correct outcome — an outage is recoverable, a divergence is not. |
| **Post-mortem** | Verify the crashed node wrote nothing, by comparing the state of both volumes after repair. `nodes_written == 2` fails the run outright, whatever else happened (SC-004). |
| **Measures** | `nodes_written`, `writes`, divergence check, crash confirmation on the isolated side. |

This is the scenario that proves the design is safe, not merely that it works.

---

## Mode-specific scenarios

The expectations of the scenarios below are written by the phase that implements the mode.
Their numbers, names and scope are fixed here.

| # | Scenario | Modes | Scope |
|---|---|---|---|
| 13 | Degraded storage | all | The process is alive and the database is unreachable. The application check returns *degraded*, which is a distinct outcome from *dead*; the documented behaviour and the number of consecutive degraded evaluations before the node is treated as dead are results of the bench. |
| 14 | Binary update | all | Updating the Maintenant binary on a running highly available deployment, without losing the service and without losing an acknowledged write. |
| 15 | Standby absent while writes continue | `postgres/streaming` | The configured conduct when the standby is missing: either block writes, or continue without a standby while announcing the loss window. Return to synchronous replication only after a full catch-up. |
| 16 | Former primary returns and is rebuilt | `postgres/streaming` | The node that was overtaken comes back. It is never restarted as a primary; it is rebuilt as a standby without intervention. |
| 17 | Write-ahead log retention exhausted | `postgres/streaming` | Retention for an absent standby is bounded. Crossing the bound triggers a documented full rebuild, never a failure of the primary. |
| 18 | Forced promotion | `postgres/streaming` | Promotion requested while the standby is behind; exactly one primary must be established before the application starts. |
| 19 | Full giveback | `postgres/streaming` | Return to nominal on operator request only, refused while the preferred node's copy is not up to date. |
| 20 | SQLite journal recovery after crash | `sqlite/drbd` | The journal left behind by an abrupt loss is recovered on the surviving node before the application is allowed to start. |
| 21 | Volume resynchronisation on node return | `sqlite/drbd` | The returning node resynchronises from the survivor without overwriting an acknowledged write, and does not restart the service on its own. |
| 22 | Storage split-brain and its manual resolution | `sqlite/drbd` | Automatic resolution is forbidden: it throws writes away. The bench must show the refusal to promote, name the side to keep, and document the manual procedure. |
