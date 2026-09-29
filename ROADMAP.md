# Roadmap

Where maintenant is going, and what it deliberately will not become.

This file is the public view. It is written from the working specifications, the git
history and the code itself, not from intentions: a line marked **Shipped** has code on
`main` you can read today. Dates after the current quarter are targets, not promises.

**Last updated**: 2026-09-08 &middot; **Current release**: v1.2.12

## The two principles that constrain everything below

1. **One binary, one SQLite file, zero external dependencies.** No Redis, no message
   queue, no mandatory Postgres. Every feature on this page has to survive that
   sentence. Where a heavier setup is needed (PostgreSQL, high availability), it is an
   opt-in operational choice that costs nothing to the person who does not want it.
2. **The data never leaves your server.** Observation is read-only, no agent runs inside
   your workloads, and there is no outbound call you did not ask for.

## Status legend

| Mark | Meaning |
|---|---|
| **Shipped** | On `main`, released |
| **In flight** | Branch exists, code partially written |
| **Planned** | Specified or scheduled, no code |
| **Considered** | Wanted, not scheduled, no commitment |

---

## Shipped

The monitoring base, in production today.

| Area | What it gives you | Edition |
|---|---|---|
| Runtime discovery | Docker, Swarm and Kubernetes detected at boot, containers inventoried without configuration | Community |
| Endpoints, heartbeats, certificates | HTTP/TCP checks from container labels, cron heartbeats, TLS expiry and OCSP revocation | Community |
| Resource metrics | CPU, memory, network and disk per container and per host, with tiered history | Community |
| Log viewer | Full-screen logs, search, level filtering, JSON highlighting | Community |
| MCP server | stdio and Streamable HTTP, full OAuth2, so an assistant can read your infrastructure | Community |
| Multi-host agents | Remote agents report their runtimes to a central server over gRPC | Pro |
| Update intelligence | OCI registry digests, OSV.dev CVE mapping, risk scoring, changelogs, compose-aware update and rollback | Pro |
| Alerting | Channels decoupled from routing rules, escalation policies, maintenance windows, silences. Webhook and Discord in Community; Slack, Teams, email and escalation in Pro | Community / Pro |
| Telegram channel | Bot-token channel, secret never shown again in clear | Pro |
| Status page | Public page, application components, incidents, maintenance, subscriber notifications, live updates over SSE | Community / Pro |
| Security posture | Container hardening analysis and a single score per container and per infrastructure | Pro |
| Interface | Design system, dense list and search views on every monitor page, light and dark theme | Community |
| PostgreSQL server store | Server data on an operator-supplied PostgreSQL. Opt-in by DSN, server mode only. SQLite stays the default and the agent's only store | Team (preview) |
| Personal edition | Lifetime non-commercial licence, lifting the Community caps | Personal |

## In flight

### Anomaly detection (Pro)

Flags what leaves a series' own normal behaviour, with no absolute threshold to set:
seasonal median absolute deviation, memory drift, change points, Theil-Sen trends,
incident correlation, and a feedback loop. Pure Go, computed locally, nothing sent
anywhere.

Code is written on its branch, tests are the last lock before release.

### Active/passive high availability on OpenSVC

Two machines, one service address, a cluster manager that notices the active node died
and restarts the instance on the passive one, with no acknowledged write lost and no
human involved.

The binary elects nobody, detects no failure and moves nothing: orchestration lives
outside it, in OpenSVC and in Ansible roles. The product's own change set is a closed
list of seven bounded points (durability, a single state root, configurable telemetry,
gRPC keepalive, heartbeat startup grace, monitor re-enrolment, refusal to start on an
empty data set), all opt-in and all inert on an existing install.

Two data mechanisms, one per backend: PostgreSQL streaming with standby promotion, and a
DRBD replicated volume for SQLite, which is the mode that matters, because SQLite is the
product's default and that combination is the documentation gap this work exists to fill.

The proof is a bench a third party can replay: two nodes, a quorum arbitrator, an
external measurement machine, three networks, twelve fault scenarios as Ansible roles,
and timestamped RTO/RPO measurements.

Target: **30 April 2027**.

### Test pyramid

Unit, integration and end-to-end coverage across the three runtimes. The base is on
`main`; the multi-runtime end-to-end harness and its dedicated CI workflow are still on a
branch. It has been in that state since April 2026.

## Planned

### Agent retention while the server is unreachable

Multi-host works, and has for a while. What it lacks is retention: when an agent cannot
reach the server, nothing it would have measured is kept.

What happens today:

- The agent reconnects on its own, retrying after 1 s, then 2, 4, 8, up to a minute
  between attempts. The server marks it offline after 60 seconds, so the dashboard shows
  the host as unreachable rather than pretending it is fine.
- On reconnection the server asks for the full container inventory and replaces what it
  had, so the current state is correct again, including containers deleted in the
  meantime.
- Everything that happened in between is gone. A container that restarted twice looks
  like it never moved. CPU, memory, network and disk graphs have a flat hole.
- Worse, endpoint and TLS checks are run by the agent, because only the agent can reach
  those targets from its own network. They stop when the connection to the server stops.
  So a site behind that agent can go down and come back up during the outage without ever
  being checked, and without raising an alert.

Two things to fix, in order:

1. **Keep checking when the server is gone.** Endpoint and certificate checks must run and
   evaluate their alert rules on the agent, whether or not the uplink is up. Without this
   there is nothing to retain in the first place.
2. **Store and resend.** Write what the agent produces while offline to its local data
   directory, resend it in order once the connection is back, and have the server confirm
   how far it has read so nothing is sent twice or dropped.

The wire format already has the sequence number and the acknowledgement field for this,
both currently unused. The agent already has a local directory that operators mount as a
volume. Nothing new gets installed: the buffer is a file on the agent, not a message
broker.

Still to decide: how much to keep (by size, by age, or both), what to drop first when that
limit is reached, in what order to resend across event kinds, and what to guarantee. For
scale, 30 containers over a one-hour outage is roughly 11,000 samples.

### Identity and access

Local users and authentication, OIDC, organisations and roles. Today authentication is
delegated entirely to the reverse proxy through ForwardAuth, which cannot express "this
user sees this subset". This is the groundwork the two items below stand on.

### Biscuit tokens

Ed25519 tokens with Datalog policies, offline attenuation and delegation: hand someone a
token that can do strictly less than yours, without a round trip to an authorisation
server. Plus the UI to issue, attenuate and revoke them.

### Multi-tenant and RBAC

Organisations with real isolation of visibility and configuration, roles, and an audit
log traceable per user and per organisation. For MSPs and platform teams running one
instance for several customers.

### OpenSVC as a monitoring source

Auto-discovery of OpenSVC services and their state through the agent's REST API, so a
cluster supervises itself in the same pane as everything else.

### Smaller planned items

Host disk exhaustion forecasting &middot; SLO and uptime reports per customer &middot;
web push through the PWA &middot; a Terraform provider &middot; an external security
audit.

## Considered

No date, no commitment: on-call schedules and rotations, per-user contact channels, voice
call escalation, a database health module, and alerting on log patterns.

## Editions

| Edition | Price | For whom |
|---|---|---|
| **Community** | Free, AGPL-3.0 | Everyone. The full monitoring base, self-hosted |
| **Personal** | One-off, lifetime | Homelabs and personal use, non-commercial. Lifts the Community caps |
| **Pro** | Monthly | Professional use. Multi-host, update intelligence and CVEs, the full alerting engine, Swarm and Kubernetes dashboards, security posture, anomaly detection |
| **Team** *(planned, 2027)* | Monthly | Teams that operate, not just observe. Identity, OIDC, PostgreSQL, server failover |
| **Enterprise** *(planned, 2027)* | Monthly | MSPs, platform teams. Team plus multi-tenancy, RBAC and audit log |

The source of every funded deliverable is published under AGPL-3.0. What is sold is
activation, in the manner of GitLab CE/EE, not access to the code.

## Non-goals

- **Becoming a metrics database.** maintenant replaces a Prometheus stack for
  infrastructure monitoring; it does not expose a Prometheus scrape endpoint, and there is
  no plan to integrate with one.
- **An agent inside your workloads.** Observation stays read-only and outside.
- **Making high availability the default.** It is an operational option, it does not touch
  the product's code, and it will never be a prerequisite for a `docker run`.
- **A mandatory external dependency**, for any feature on this page.

---

Part of this roadmap is carried out with the support of France 2030.

Corrections and disagreements are welcome as issues: a roadmap that nobody argues with is
usually a roadmap nobody read.
