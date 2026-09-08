# High availability: bench and reference deployment

This directory holds everything needed to build a two-node highly available Maintenant
deployment, and to prove it works: the Ansible roles, the OpenSVC service templates, the
local test bench and the versioned run reports.

The same roles deploy the bench and the reference production. `site-lab.yml` and
`site-prod.yml` differ only by their inventory and by the fencing variable — nothing else.
No throwaway script is ever required to deploy.

## Layout

| Path | Contents |
|---|---|
| `ansible/` | roles, `ansible.cfg`, inventories (`lab`, `prod`), shared versions |
| `opensvc/` | service templates, one per replication mode |
| `lab/` | bench topology, provisioning, `lab` CLI, scenarios and probes |
| `lab/scenarios/` | the twelve scenarios of the bench plan, plus the mode-specific ones |
| `lab/probes/` | the two measurement probes (availability, writes) |
| `reports/` | versioned run reports, one directory per run |

## Where the authority lives

- Design and build order: [`specs/030-ha-opensvc/plan.md`](../../specs/030-ha-opensvc/plan.md)
- Requirements: [`specs/030-ha-opensvc/spec.md`](../../specs/030-ha-opensvc/spec.md)
- Contracts: [`specs/030-ha-opensvc/contracts/`](../../specs/030-ha-opensvc/contracts/) —
  the `lab` CLI, the run report format, the OpenSVC service, the binary settings
- Operator guide: `docs/guides/high-availability.md` (to be written, FR-054)

## Vocabulary

- **Measurement machine**: the fourth VM of the bench, outside both sites.
- **Probe**: one of the two measurement processes that machine runs (availability, writes).
  A probe is never the machine.
