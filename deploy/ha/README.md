# High availability: bench and reference deployment

This directory holds everything needed to build a two-node highly available Maintenant
deployment, and to prove it works: the Ansible roles, the OpenSVC service templates, the local test bench and the versioned run reports.

The same roles deploy the bench and the reference production. `site-lab.yml` and
`site-prod.yml` differ only by their inventory and by the fencing variable, nothing else.
No throwaway script is ever required to deploy.

## Layout

| Path                                | Contents                                                                                                                            |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `ansible/`                          | roles, `ansible.cfg`, inventories (`lab`, `prod`), shared versions                                                                  |
| `ansible/roles/opensvc/`            | om3 on the frozen release, and the cluster the nodes form                                                                           |
| `ansible/roles/opensvc_arbitrator/` | the third-site witness, which withholds its vote from a DRBD copy behind a connected primary, and the quorum settings the nodes hold |
| `ansible/roles/fencing/`            | the callout a survivor runs to shoot its unreachable peer                                                                           |
| `ansible/roles/maintenant/`         | the static binary, the state root, the settings and, in server mode, the gRPC pair, one table for both the env file and the service |
| `ansible/roles/maintenant_check/`   | the application test the service runs: 0 alive, 1 degraded, 2 dead                                                                  |
| `ansible/roles/postgres_ha/`        | `postgres/streaming`: PostgreSQL primary and synchronous standby, promotion, anti-return lock, rebuild of the former primary        |
| `ansible/roles/drbd/`               | `sqlite/drbd`: DRBD 9 from the LINBIT packages, protocol C, flushes on, a split brain that disconnects and never discards a side, a diskless quorum vote on the arbitrator that freezes the writes of an isolated node |
| `ansible/roles/opensvc_service/`    | renders the mode's template, declares the service and waits for it to be up on one node                                             |
| `ansible/roles/maintenant_agent/`   | an ordinary agent as a systemd unit on each node and on the measurement machine, enrolled with a one-time token                      |
| `ansible/roles/lab_witness/`        | Docker and the witness container whose resource history counts the samples a failover lost                                           |
| `ansible/roles/probes/`             | the three probes, built, shipped and kept running on the measurement machine                                                        |
| `opensvc/`                          | service templates, one per replication mode                                                                                         |
| `split-brain.md`                    | the manual resolution of a DRBD split brain in `sqlite/drbd`, whose side to keep `lab status` names                                  |
| `lab/`                              | bench topology, provisioning, `lab` CLI, scenarios and probes                                                                       |
| `lab/lab`                           | the single entry point: `up`, `down`, `status`, `chaos`, `heal`, `run-all` (one scenario with `--scenario <n>`), `giveback`           |
| `lab/scenarios/`                    | the twelve scenarios of the bench plan, plus the mode-specific ones                                                                 |
| `lab/probes/`                       | the measurement probes (availability, writes, observer), in Go                                                                      |
| `reports/`                          | versioned run reports, one directory per run                                                                                        |

## Where the authority lives

- Design and build order: [`specs/030-ha-opensvc/plan.md`](../../specs/030-ha-opensvc/plan.md)
- Requirements: [`specs/030-ha-opensvc/spec.md`](../../specs/030-ha-opensvc/spec.md)
- Contracts: [`specs/030-ha-opensvc/contracts/`](../../specs/030-ha-opensvc/contracts/):
  the `lab` CLI, the run report format, the OpenSVC service, the binary settings
- Operator guide: `docs/guides/high-availability.md` (to be written, FR-054)

## Driving the bench

```sh
deploy/ha/lab/lab up --mode sqlite/drbd
deploy/ha/lab/lab status --json
deploy/ha/lab/lab down
```

It reaches the machines through the Ansible inventory and nothing else, so replaying the campaign elsewhere means pointing `LAB_INVENTORY` at another inventory. `status` exits 0 only when the bench is green, which is what makes `up` self-checking.

The host needs KVM with nested virtualisation (check with `kvm-ok`), libvirt, `qemu-img`,
an ISO builder (`genisoimage` or `xorrisofs`), `jq`, `ssh`, `ansible-core`, the python
libvirt binding and the `community.libvirt` collection:

```sh
sudo apt install qemu-kvm libvirt-daemon-system python3-libvirt qemu-utils xorriso jq
ansible-galaxy collection install -r deploy/ha/ansible/requirements.yml
```

The bench runs as the invoking user, never under `sudo`. That user belongs to the `libvirt` group and has an SSH key pair, `~/.ssh/id_ed25519` unless `LAB_SSH_KEY` names another private key (its `.pub` beside it): cloud-init installs that key on the four machines and the bench reaches them with it.

What an ordinary user cannot do is prepared once, with `sudo`, which asks for your password once (run it as yourself, not under `sudo`):

```sh
deploy/ha/lab/lab host-setup
```

It creates the two image directories (`/var/lib/libvirt/images/mnt-lab` and `mnt-lab-base`, owned by the user), the `maintenant-fence` account, the callout that account is restricted to and its key, kept in `~/.local/share/maintenant-lab`. It is idempotent, and `down` leaves all of it in place. `lab up` checks each of these before touching anything and says to run `host-setup` when one is missing.

`up` builds the three networks and the four machines from `lab/topology.yml`, which is the single source of addresses, sizes and images. Each machine boots a qcow2 overlay on a cached Debian cloud image, configured by a cloud-init seed with static addresses matched by MAC. The two cluster nodes get a second disk, `/dev/vdb`, which is where the replicated state lives: DRBD backs onto it in `sqlite/drbd`, PostgreSQL puts its data directory there in the streaming modes. `down` undefines every domain and network and deletes the volumes; only the cached base image survives, being an upstream artefact rather than bench state.

Modes are `ip-only` (the default: service address failover, no replicated backend),
`postgres/streaming`, `sqlite/drbd` and `postgres/drbd`.

## The frozen release

`ansible/group_vars/all/versions.yml` is the only place a version is chosen, and the
`opensvc` role refuses to go further if the node ends up running anything else: every
measurement is attached to a release, so a silent upgrade would quietly invalidate the
evidence. apt is pinned to the exact package version for the same reason.

The bench runs om3 **v3.0.0-rc62 from the prod branch, on Debian 12**, the newest release that branch ships with `opensvc-server` for bookworm. It ran rc47 from 2026-10-27 at OpenSVC's request, until the prod repository dropped every release older than rc55 (2026-11-11). As of 2026-10-26 it still ships only `opensvc-client` for trixie, so Debian 13 cannot host a cluster node. The uat branch is not used: evidence built on a pre-validation channel is worth less than evidence built on the channel OpenSVC promotes.

The campaign ran on rc30 until 2026-10-18. rc53 replaces it because the deployment now relies on two things rc30 does not have: `om <path> create --wait`, which returns once every daemon knows the object, and `om cluster enroll`, which adds a node from inside the cluster. rc53 also changes what the roles wait for: a created object is no longer frozen but flagged *stopped on purpose*, and only a start asked by the user lowers that flag, so the role ends the deployment with `om <path> start --wait`. Every report before 2026-10-26 was taken on rc30.

## Joining the cluster

The nodes join by enrollment, the procedure the om3 book describes for rc47 and later. The joining node creates a token with the join role (`om daemon auth --role join`), and the forming node runs `om cluster enroll --node <joining node> --wait`, which returns once the new node's heartbeat beats in the cluster. The token reaches the command through `OSVC_JOIN_TOKEN`, never on a command line. A node is enrolled by its name, which `/etc/hosts` pins on every node: the certificate om3 generates names the node, not its interconnect address.

Removing a node is the reverse: drain it (`om node drain --node <node> --wait`), then evict it from a node that stays (`om cluster evict --node <node> --wait`). An evicted node restarts as a single-node cluster with a secret of its own. The bench never removes a node; a failed node is repaired in place.

## The arbitrator

om3 ships `cluster.quorum` **off**. With it off, a split cluster logs "cluster is split,
ignore" and both sides start the service, which is the one outcome the bench exists to rule out. The `opensvc_arbitrator` role turns quorum on and declares the witness in `cluster.conf`, sets `node.split_action=reboot` on each node, then waits until every daemon reports quorum on, the witness up and the split action set, and fails the run otherwise.

`cluster.quorum` has to go through `om cluster config`. om3 also accepts it in `node.conf` and reads it back from there, but the split decision only reads `cluster.conf`; the arbitrators are likewise reloaded only when the cluster configuration changes. A value read back from a file therefore proves nothing: the role and `lab status` read the daemon's own view.

The witness is an HTTP endpoint on the third machine, port 1216, path `/arbitrator`. The port is written out in full and always will be: up to rc30 the keyword reference said a portless TCP arbitrator uri implied port 1214 while the CHANGELOG said 1215. Since then the reference says it is the port of the cluster listener, `listener.port`, 1215 by default. An implicit port would still tie the vote to a setting of another component (FR-021).

### How om3 counts the vote

om3 rc47 asks for the vote with a GET on `arbitrator#0.uri`. `checkURL` (`daemon/nmon/arbitrator.go`, line 148) returns the error of `client.Do` and never looks at the HTTP status: any response is a vote, a 403 or a 500 included. Only a transport error withholds it: connection refused, connection closed before any response (the client sees EOF), a TLS failure, or the check timeout. That timeout is `arbitratorCheckDuration`, a third of the daemon's ready duration of 5 s, about 1.67 s.

A node fetches the vote live when it forgets a peer, in `onForgetPeer` (`daemon/nmon/main_cmd.go`, line 362, votes read at line 404). That happens `hb.timeout` (15 s by default) after the last heartbeat it received, heartbeats being sent every `hb.interval` (5 s by default), so between 10 and 15 s after the link is cut. Both nodes ask, each on its own timer. Each counts its live peers, itself included, plus the arbitrator votes, out of the cluster nodes plus the arbitrators. With two nodes and one arbitrator, a node alone needs the vote to reach 2 of 3; without it, it runs `node.split_action`, `crash` here. The same check also runs every 60 s, for status display only.

### Where the witness stands

The om3 keyword reference of `arbitrator.uri` places the arbitrators on the side of one segment: "In case of a real split, all arbitrators are expected to be unreachable from the lost segment. At least one of them is expected to be reachable from the surviving segment", and "Usually, this can be interpreted as: same site, not same rack and power lines." With that layout, a vote that answers whoever reaches it is enough: the segment that cannot reach the arbitrator loses.

This deployment puts the witness on a third site instead. With two nodes, a witness on site A means the loss of site A takes the witness with it, and the node on site B cannot reach quorum: failover is automatic in one direction only. A third-site witness lets the survivor of either site win the vote. The price is the inter-site partition (scenario 5): both segments still reach the witness, so a witness that grants every GET gives quorum to both. In `sqlite/drbd` the passive node then takes over and fences the active node, the only one with an `UpToDate` copy. The `Outdated` refusal of the fencing callout (below) stops that shot, and the witness itself now refuses the vote in that case.

### The selective vote

The `opensvc_arbitrator` role runs a small Python 3 HTTP server (standard library only, `files/arbitrator.py`) as the systemd unit `opensvc-arbitrator`, in place of the nginx site the role used to install. For each GET on the vote path it identifies the requester by its source address, mapped to its inventory name from the `ansible_host` of each node of `opensvc_cluster`. If `drbdsetup status --json <resource>` answers on the third machine, which is the diskless DRBD vote in `sqlite/drbd`, it refuses the requester if and only if:

- the requester's `peer-disk-state`, as the third machine sees it, is not `UpToDate`, and
- another peer is `connection-state: Connected`, `peer-role: Primary` and `peer-disk-state: UpToDate`.

Every other case is granted with a 200, as before: an unknown source address, no DRBD on the machine (`ip-only`, `postgres/streaming`), a requester absent from the resource, or a `drbdsetup` call that fails or exceeds 0.5 s. The rule only acts in the case it can decide. Refusing means closing the connection without writing any response, so that om3 gets EOF and counts no vote; a 403 would count as one in rc47. Each request leaves one line in the journal: source address, node, decision, reason.

Applied to scenario 5, the primary is `UpToDate` and gets the vote; the cut secondary is `Outdated` while the primary is still connected to the third machine, gets none, lacks quorum and reboots. While the partition lasts it comes back, is refused again and reboots again, which is harmless: its disk is `Outdated`, so neither DRBD, the arbitrator nor `maintenant-fence --guard` lets it start the service. Once the partition heals it rejoins and resyncs on its own. `crash`, the other self-fencing action, leaves the node hung after the panic (`kernel.panic=0` on Debian) until someone power cycles it. When the primary is really lost, the third machine no longer sees a connected primary and the survivor gets the vote, whatever its disk state.

### Timing

The rule reads DRBD's view at the moment of the GET, so DRBD must have noticed the cut, and outdated the cut side, before om3 asks. On an idle connection DRBD notices a silent peer after at most `ping-int` + `ping-timeout`, 10 s + 0.5 s with the defaults: about the same time as om3's earliest detection. Run `20261027-180813-sqlite-drbd`, played with the `Outdated` refusal disabled, shows how thin that margin is:

| Time (UTC) | Event |
|---|---|
| 18:08:19.67 | partition injected |
| 18:08:30.107 | DRBD on site-b detects the cut |
| 18:08:30.122 | site-b goes `Outdated` |
| 18:08:30.137 | `arb` logs `site-b: pdsk( UpToDate -> Outdated )` |
| 18:08:33.714 | om3 on site-b: `cluster is split, we have quorum: 1+1` |

The third machine knew who was `Outdated` 3.6 s before om3 asked, by luck of the heartbeat phase. The `drbd` role therefore sets `ping-int` to 3 s (`drbd_ping_int_s`, with `ping-timeout` kept at its default of 0.5 s, `drbd_ping_timeout_ds`) and refuses to deploy unless `ping-int + ping-timeout` stays below `hb.timeout - hb.interval`. These roles do not set the om3 heartbeat; `group_vars/all/opensvc.yml` declares its defaults, 15 s and 5 s, for that check.

### Limits

- In `ip-only` and `postgres/streaming` the third machine has no DRBD and every vote is granted: an inter-site partition still gives quorum to both segments. A rule reading the PostgreSQL replication state is the next step.
- A node is recognised by the address it reaches the witness from, which must be its `ansible_host`, and its DRBD peer name (its hostname) must equal its inventory name, as on the bench. Otherwise its vote is always granted.
- If om3 asked before DRBD noticed the cut, both copies would still read `UpToDate` and both nodes would get the vote. The timing check above is what rules that out.
- A node resyncing (`Inconsistent`) while the primary is connected gets no vote, in the periodic check too: its status shows the arbitrator down until the resync ends, and the role's reachability check fails if a deployment runs during a resync.

### Proposed upstream

- Count only a 2xx response as a vote, and close `resp.Body`, which `checkURL` leaves open.
- Tell the periodic check from a split vote, and send the node name with the request, so that an arbitrator can decide without mapping addresses.
- Have the takeover that follows the loss of a peer consult the DRBD driver's start blocker, as the switch does in `imon/wait_destination.go`. Both appear between rc47 and rc53: in rc47 the switch goes through the driver's `PreMove`, and the takeover consults neither.

## Fencing

Before a survivor starts a service its peer was running, it shoots that peer. The `fencing` role installs the callout and declares it as `stonith#<node>.command` on every node. On the bench, the part that lives on the libvirt host (account, callout, key) is installed by `lab host-setup`; the role only checks it is there.

The mechanism itself is two inventory variables and nothing else, which is what lets the
same role serve the bench and a real deployment: `ha_fencing_command` shoots,
`ha_fencing_check_command` proves the mechanism answers. On the bench the nodes reach the libvirt host over SSH with a key restricted to a single forced command, and that command destroys the peer's domain; elsewhere the same two variables carry IPMI, a PDU or a provider API. Both are required: a deployment with no fencing is refused, not warned about.

Two things never happen quietly. A fence that cannot prove the peer went down exits
non-zero. om3 only logs that and starts the service anyway, so the callout also leaves `/var/lib/maintenant-fence/failed-<node>` behind, and `maintenant-fence --guard`, the blocking pre-start of `ip#0`, refuses to start the service while such a file exists (FR-024). A later successful fence of the same node removes it; otherwise the operator makes sure the node is down, removes the file and starts the service. And the role exercises the mechanism at deploy time with the check command: a cluster should not discover during an incident that it was never able to fence.

In `sqlite/drbd`, a node whose DRBD copy is `Outdated` neither fences nor starts, and leaves no mark. The peer that outdated it is alive and kept the DRBD quorum through the diskless vote, so it keeps the service. This is the inter-site partition (scenario 5): both nodes still reach the witness, and if the witness granted the vote to both sides, the passive node would shoot the only node holding the data. The selective vote (see The arbitrator) normally withholds it first; this refusal stays as a second barrier.

`DEFAULT.stonith=true` is the other half, and it lives on the service rather than the node: a failover service without it never calls the callout at all. The service templates carry it.

### Power off or power cycle

The bench fences by power off: `virsh destroy` leaves the peer down until someone starts it, and every measure in `reports/` was taken that way. A production deployment can choose a power cycle instead, as most fence agents do by default: the fenced node boots again, rejoins the cluster and resyncs from the survivor without anyone going to the machine.

A power cycle is still a power off first. `ha_fencing_command` must exit 0 only once the peer is confirmed off, and a failed power on must not fail the fence, since the peer did go down. `ipmitool chassis power cycle` alone does not prove that: it returns as soon as the BMC accepts the request. Write the three steps out:

```yaml
ha_fencing_command: >-
  ipmi() { ipmitool -H "$ha_fencing_target-bmc" -U "$IPMI_USER" -P "$IPMI_PASSWORD" "$@"; };
  ipmi chassis power off || exit 1;
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    ipmi chassis power status | grep -q 'is off' && { ipmi chassis power on || :; exit 0; };
    sleep 1;
  done;
  exit 1
```

Two consequences to accept before choosing it:

- The fenced node is back within a minute, every time. It must not start the service on its own and its copy must resync without overwriting an acknowledged write. om3 does not start a failover service already running elsewhere and `--guard` refuses a start after a failed fence, but this return has not been played on the bench yet.
- During an inter-site partition where both nodes reach the witness, the rebooted node comes back still cut from its peer, holds the quorum through the witness, and can fence the survivor in turn. In `sqlite/drbd` the `Outdated` refusal above stops this loop. In `postgres/streaming` and `ip-only` nothing does yet, so power off stays the safer choice for these modes.

## Support data

Every cluster node carries `sos`, and journald writes to disk every second instead of every five minutes, so a fenced node keeps its last logs. When a bench verification fails, `lab` keeps om3's view (`om mon`, `om <service> status`) and a sosreport of each node in the report, under `om/` and `sos/`; a node that is down at that moment gets its sosreport after the repair. The sosreport is the one OpenSVC support asks for, `sos report --enable-plugins opensvc --all-logs`. Its OpenSVC plugin copies `/var/lib/opensvc/sec`, the private key of the cluster CA included: look at what an archive holds before it leaves the bench.

## The loss window of `synchronous=NORMAL` in `sqlite/drbd`

The role refuses `sqlite/drbd` below `MAINTENANT_SQLITE_SYNCHRONOUS=FULL` (FR-017). At `NORMAL`, SQLite acknowledges a commit before the write-ahead log reaches the disk, so DRBD never sees it when the node dies, and the survivor starts without it.

Measured on 2026-10-18 by playing scenario 2 (`virsh destroy` of the active node) 20 times in `sqlite/drbd`, with `NORMAL` set by hand in `app#0.environment` and the writes probe at one record every 100 ms:

| | per repetition, over 20 |
|---|---|
| Acknowledged writes lost | 3 to 7 (7 in 12 runs, 6 in 7 runs, 3 in one) |
| Loss window | 0.3 to 0.7 s, 0.7 s in 12 runs |
| Service back on the address | 22.6 to 26.0 s |

The window is the time the lost writes cover, at 100 ms per write, read from the probe's journal. Every repetition lost writes, and they were always the last ones acknowledged before the crash, contiguous, never an older or isolated record. Nothing else failed. The same scenario at `FULL` loses nothing (SC-002). The figure holds for this write rate on the bench machines; the bench does not show what bounds it, so a heavier load or slower storage can widen it.

To replay the measurement, set `NORMAL` in the service environment and restart the application, then run `lab run-all --scenario 2 --repeat 20`. The role's guard does not allow it, so it is changed outside Ansible, and set back to `FULL` afterwards:

```sh
v=$(sudo om maintenant config get --kw app#0.environment | sed s/SYNCHRONOUS=FULL/SYNCHRONOUS=NORMAL/)
sudo om maintenant config update --set "app#0.environment=$v"
sudo om maintenant restart --rid app#0
```

## The probes

All three run on the measurement machine, outside both sites, and both keep running from `lab up` to `lab down`: no scenario ever restarts them. They are Go programs built like the product binary is, and CI compiles and lints them with the rest of the backend, because every figure in a run report comes out of them.

**availability** issues one request every 100 ms on the service address and writes one ndjson line per request. Each line carries a wall clock stamp, to line up with the other logs, and a monotonic offset, which is the one that can be trusted to measure a duration. The cadence is held even while requests hang, since an outage is exactly when they do.

**writes** sends numbered records through the API and journals what the server answered for each: acknowledged, refused, or unknown when no answer came back. After recovery it reads 
every record back and compares. Only acknowledged records that are missing count as loss. A write that never left the machine is refused and cannot be a loss; a write that left and got no answer is unknown, counted neither as loss nor as success, because nobody can say whether it committed.

The record is a heartbeat ping: the pings table is append-only, uncapped, and is the busiest write path the product has, so the instrument writes the way the product does. The server answers the id of the ping it stored; the journal keeps it and the read-back looks for it. The product never stores a ping body, so the id is the only handle on a record. The heartbeat the role creates has a period of a day, so it can never fall overdue inside a scenario and can never show up among the alerts the failover caused.

**observer** polls the telemetry and the active alerts once a second, and produces the two figures that are reported beside the write loss and never mixed into it. The telemetry hole is a duration plus the number of samples missing from the stored history, counted against the cadence read from that history rather than an assumed one. A hole is not data loss: samples were never collected because the collector was moving, and counting them as lost writes would be a lie about the RPO. An alert counts as induced only when its subject was healthy just before the injection, so a monitor that was already failing does not become the failover's fault; without a reading from before the injection the summary refuses rather than blame everything on the failover.

The observer also reports whether the agents came back. The bench enrolls an ordinary agent on site-a, site-b and the measurement machine, which also runs a witness container whose stored history the samples are counted on. The agents expected back are those available just before the injection, minus the nodes that were down: a node whose boot id changed or could not be read during the repetition. Each expected agent must be available again within 90 s, under the same identity. An agent that comes back under a new id has re-enrolled, and that fails the repetition.

`run-all` fails a repetition when a measure the scenario declares comes back empty or in error, rather than reporting it as n/a. A declared measure the bench cannot take yet is reported as `not_measured`.

All three units restart on their own if they die, and a probe picks up the numbering where its journal left off, so a crash costs a few records rather than the whole comparison. Each line also carries the `epoch` of the process that wrote it: `mono_ns` measures a duration exactly within one epoch, and across a restart only the wall clock is common ground.

## Vocabulary

- **Measurement machine**: the fourth VM of the bench, outside both sites.
- **Probe**: one of the two measurement processes that machine runs (availability, writes). A probe is never the machine.
