# High availability: bench and reference deployment

This directory holds everything needed to build a two-node highly available Maintenant
deployment, and to prove it works: the Ansible roles, the OpenSVC service templates, the local test bench and the versioned run reports.

The same roles deploy the bench and the reference production. `site-lab.yml` and
`site-prod.yml` differ only by their inventory and by the fencing variable — nothing else.
No throwaway script is ever required to deploy.

## Layout

| Path                                | Contents                                                                                                                            |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `ansible/`                          | roles, `ansible.cfg`, inventories (`lab`, `prod`), shared versions                                                                  |
| `ansible/roles/opensvc/`            | om3 on the frozen release, and the cluster the nodes form                                                                           |
| `ansible/roles/opensvc_arbitrator/` | the third-site witness, and the quorum settings the nodes hold                                                                      |
| `ansible/roles/fencing/`            | the callout a survivor runs to shoot its unreachable peer                                                                           |
| `ansible/roles/maintenant/`         | the static binary, the state root, the settings and, in server mode, the gRPC pair, one table for both the env file and the service |
| `ansible/roles/maintenant_check/`   | the application test the service runs: 0 alive, 1 degraded, 2 dead                                                                  |
| `ansible/roles/postgres_ha/`        | `postgres/streaming`: PostgreSQL primary and synchronous standby, promotion, anti-return lock, rebuild of the former primary        |
| `ansible/roles/opensvc_service/`    | renders the mode's template, declares the service and waits for it to be up on one node                                             |
| `ansible/roles/maintenant_agent/`   | an ordinary agent as a systemd unit on each node and on the measurement machine, enrolled with a one-time token                      |
| `ansible/roles/lab_witness/`        | Docker and the witness container whose resource history counts the samples a failover lost                                           |
| `ansible/roles/probes/`             | the three probes, built, shipped and kept running on the measurement machine                                                        |
| `opensvc/`                          | service templates, one per replication mode                                                                                         |
| `lab/`                              | bench topology, provisioning, `lab` CLI, scenarios and probes                                                                       |
| `lab/lab`                           | the single entry point: `up`, `down`, `status`, `chaos`, `heal`, `run-all`, `giveback`                                              |
| `lab/scenarios/`                    | the twelve scenarios of the bench plan, plus the mode-specific ones                                                                 |
| `lab/probes/`                       | the measurement probes (availability, writes, observer), in Go                                                                      |
| `reports/`                          | versioned run reports, one directory per run                                                                                        |

## Where the authority lives

- Design and build order: [`specs/030-ha-opensvc/plan.md`](../../specs/030-ha-opensvc/plan.md)
- Requirements: [`specs/030-ha-opensvc/spec.md`](../../specs/030-ha-opensvc/spec.md)
- Contracts: [`specs/030-ha-opensvc/contracts/`](../../specs/030-ha-opensvc/contracts/) —
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

The bench runs om3 **v3.0.0-rc30 from the prod branch, on Debian 12**. The state of the art had recorded rc31, which turns out to be installable nowhere: packages.opensvc.com`
publishes no rc31 on any branch. The prod branch ships `opensvc-server` for bookworm up to rc30 and, as of 2026-09-12, only `opensvc-client` for trixie — so Debian 13 cannot host a cluster node at all today. The uat branch does have rc32 to rc35 for trixie, but evidence built on a pre-validation channel is worth less than evidence built on the channel OpenSVC promotes.

## The arbitrator

om3 ships `cluster.quorum` **off**. With it off, a split cluster logs "cluster is split,
ignore" and both sides start the service, which is the one outcome the bench exists to rule out. The `opensvc_arbitrator` role turns quorum on and declares the witness in `cluster.conf`, sets `node.split_action=crash` on each node, then waits until every daemon reports quorum on, the witness up and the split action set, and fails the run otherwise.

`cluster.quorum` has to go through `om cluster config`. om3 also accepts it in `node.conf` and reads it back from there, but the split decision only reads `cluster.conf`; the arbitrators are likewise reloaded only when the cluster configuration changes. A value read back from a file therefore proves nothing: the role and `lab status` read the daemon's own view.

The witness is a plain HTTP endpoint on the third machine, served by nginx on port 1216, and om3 votes by issuing a GET on it. The port is written out in full and always will be: om3 implies port 1214 on a portless TCP arbitrator uri while its own listener sits on 1215, and the CHANGELOG and the keyword reference disagree about which — an implicit port would send the vote somewhere nobody chose (FR-021).

## Fencing

Before a survivor starts a service its peer was running, it shoots that peer. The `fencing` role installs the callout and declares it as `stonith#<node>.command` on every node. On the bench, the part that lives on the libvirt host (account, callout, key) is installed by `lab host-setup`; the role only checks it is there.

The mechanism itself is two inventory variables and nothing else, which is what lets the
same role serve the bench and a real deployment: `ha_fencing_command` shoots,
`ha_fencing_check_command` proves the mechanism answers. On the bench the nodes reach the libvirt host over SSH with a key restricted to a single forced command, and that command destroys the peer's domain; elsewhere the same two variables carry IPMI, a PDU or a provider API. Both are required — a deployment with no fencing is refused, not warned about.

Two things never happen quietly. A fence that cannot prove the peer went down exits
non-zero. om3 only logs that and starts the service anyway, so the callout also leaves `/var/lib/maintenant-fence/failed-<node>` behind, and `maintenant-fence --guard`, the blocking pre-start of `ip#0`, refuses to start the service while such a file exists (FR-024). A later successful fence of the same node removes it; otherwise the operator makes sure the node is down, removes the file and starts the service. And the role exercises the mechanism at deploy time with the check command: a cluster should not discover during an incident that it was never able to fence.

`DEFAULT.stonith=true` is the other half, and it lives on the service rather than the node: a failover service without it never calls the callout at all. The service templates carry it.

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
