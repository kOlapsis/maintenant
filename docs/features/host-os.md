# Host OS End-of-Support

Know when a host's Linux distribution stops receiving security updates, before it happens. Each monitored host reports its distribution and version; maintenant compares it against the support cycles of Debian, Ubuntu, RHEL, Rocky Linux, AlmaLinux, Alpine and SLES and alerts per host, the same way it already alerts on certificate expiry.

---

## How It Works

Every host — the server itself and every remote agent, binary, in a Docker container, or on a Kubernetes node — reports its operating system identity. maintenant matches it against a support-cycle table and raises a `host` / `os_eol` alert per host:

- **Warning** once 30 days or fewer remain before the security support date.
- **Critical** the day after that date passes.
- **Resolved automatically** as soon as the host reports a newer, still-supported version.

There is never more than one end-of-support alert active per host: the warning escalates in place to critical, it does not open a second alert.

---

## Identifying the Host

The identity is always read at the source, never guessed:

- **Binary agent and server binary** — read `/etc/os-release` directly.
- **In a container** — read `/host/etc/os-release`, the host's file bind-mounted read-only:

```bash
docker run … -v /etc/os-release:/host/etc/os-release:ro …
```

```yaml
volumes:
  - /etc/os-release:/host/etc/os-release:ro
```

Without the mount, the host shows as **unknown** with the reason "mount missing" — never the operating system of the container's own image. This is the same fallback pattern as the existing `/proc:/host/proc:ro` mount for resource metrics, except there is no fallback: guessing the wrong OS would be worse than showing nothing. The [Multi-Host Monitoring](multihost.md#requirements) mount snippet, the generated `docker run` and Compose commands on the Agents page, and this repository's own `compose.yml`, all include it already.

- **On Kubernetes** — no mount at all. The agent runs as a DaemonSet, one per node, and derives the identity from that node's `osImage`, which it already collects as part of its topology snapshot. The agent finds its own node by pod name, or by the `MAINTENANT_NODE_NAME` environment variable (set from `spec.nodeName` in the generated manifest) when the pod name does not match.

The agent re-reads the identity every hour and reports it again only when it changed, so an upgrade is seen within the day without restarting the agent.

**Limit**: a bind-mounted file keeps the inode it resolved at container start. An OS upgrade that rewrites `/etc/os-release` (as `dpkg` does) is only picked up after the *container* restarts — the hourly re-read cannot see it, because the mount itself is stale. This is not a concern for the binary agent, which reads the live file every time.

---

## The Three Dates, and Which One Counts

Every tracked distribution has up to three end-of-support dates:

| Date | Meaning | Used for alerting? |
|------|---------|---------------------|
| End of active support | End of new features and minor updates | No — informational only |
| End of free security support | Last day security patches ship without a paid subscription (for Debian, this is the end of LTS) | **Yes — this is the date the alert is based on** |
| End of paid extended support | ELTS (Debian), ESM (Ubuntu Pro), ELS (RHEL), LTSS (SLES) | No — shown, never alerted on |

Extended support is never used to compute the alert, because maintenant has no way to know whether you are actually subscribed to it. If you are, acknowledge the alert or silence it with a rule — that is preferable to staying silent for everyone who is not subscribed.

**Example**: Debian 11's free security support ended on 2026-08-31. The warning opens on 2026-08-01 (30 days before), and the alert turns critical on 2026-09-01 — even though Debian's paid ELTS keeps that release patched until 2031.

---

## Support States

| State | Meaning | Alert |
|-------|---------|-------|
| `supported` | More than 30 days of active support remain | None |
| `security_only` | Active support has ended; security patches still ship | None |
| `ending_soon` | 30 days or fewer of security support remain | Warning |
| `ended` | Security support has ended | Critical |
| `unknown` | No identity was reported | None |
| `untracked` | An identity was reported, but the distribution or version is not in the table | None |

`unknown` always carries a reason:

| Reason | Meaning |
|--------|---------|
| `mount_missing` | Running in a container without the `os-release` mount |
| `file_unreadable` | The file exists but could not be read or parsed |
| `node_not_found` | Kubernetes: the agent's node could not be resolved |
| `agent_too_old` | The agent has never reported an OS identity at all |

`untracked` covers distribution derivatives (Raspberry Pi OS, Linux Mint, Pop!_OS…), distributions maintenant does not follow (Fedora, Arch, NixOS, Amazon Linux, Flatcar, Talos, Bottlerocket), and versions the table does not recognize — including Debian testing/sid, which has no `VERSION_ID` to match against. A derivative's identity is displayed exactly as reported; its dates are never guessed from a parent distribution. `unknown` and `untracked` hosts never raise an alert.

---

## Where to See It

- **Agents page** — each host's distribution, version and support state, with the three dates and the unavailability reason in the detail panel.
- **Kubernetes nodes page** — the same support state, derived from each node's `osImage`.
- **Updates page** — an "Operating systems" section lists every host that is `ended` or `ending_soon`, alongside image updates; the summary card counts them.
- **MCP `get_updates`** — the `hosts` array carries every host's identity and support state, and `eol_table` carries the origin and freshness of the dates in use. See [MCP Server](mcp.md#monitoring-read).
- **Alert Triggers** — filter or route on source `host` like any other alert source.

---

## Support Table and Refresh

maintenant embeds a table of support cycles with each release, current as of its publication date. When the server has outbound network access, it refreshes the table from [endoflife.date](https://endoflife.date) once at startup and once a day after that — seven requests, one per tracked distribution family, carrying no data about your installation.

A refresh only replaces the table if all seven requests succeed and parse cleanly. Any failure — a timeout, an unexpected response, a malformed field — leaves the previous table untouched and logs a single line; it never applies a partial or empty table.

Set `MAINTENANT_DISABLE_OS_EOL_REFRESH=1` to turn the refresh off entirely, for air-gapped or network-restricted deployments. The embedded table then applies on its own, indefinitely. The origin of the dates in use (`embedded` or `endoflife.date`) and the date of the last successful refresh are exposed in the API and in `get_updates`.

---

## Out of Scope

- Pending package updates (`apt`, `dnf`, `apk`) and unapplied security patches.
- Kernel version, reboot-required detection.
- End-of-support for the base image of a *container* — that is a separate concern, tracked as part of Update Intelligence.

---

## Related

- [Multi-Host Monitoring](multihost.md) — the `os-release` mount, alongside the `/proc` mount for resource metrics
- [Update Intelligence](updates.md) — the "Operating systems" section on the Updates page
- [Alert Engine](alerts.md) — how the `host` / `os_eol` alert fits the shared alert pipeline
- [MCP Server](mcp.md) — host OS and support state via `get_updates`
