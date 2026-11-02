# Escalation bench

One disposable QEMU machine proves that an unacknowledged alert climbs a three-level
escalation chain to its end, on a real isolated host with real channels: an application
crashes, maintenant raises the alert, the policy notifies a webhook, then an on-call
mailbox, then the CTO mailbox and a pager webhook, then declares the chain exhausted.
Everything is driven by `lab`, unattended, and the verdict is computed from what the
mail and webhook catchers actually received, not from what the product says it sent.

## What runs on the machine

Debian 13 cloud image, Docker CE from the Docker repository, and `stack/compose.yml`:

| Container        | Role                                                                    |
| ---------------- | ----------------------------------------------------------------------- |
| `maintenant`     | the product under test, Pro licence, container-down threshold 30 s       |
| `mailpit`        | SMTP catcher for the email channels (UI on the host at 127.0.0.1:18025) |
| `webhook-tester` | HTTP catcher for the webhook channels (host 127.0.0.1:18081)            |
| `shop-web`       | nginx, an app that keeps running                                        |
| `shop-api`       | traefik/whoami, the app that crashes: no restart policy, alert severity label `critical` |
| `shop-cache`     | redis, another app that keeps running                                   |

The product UI is on the host at http://127.0.0.1:18080.

## Host prerequisites

`qemu-system-x86`, `qemu-utils`, `xorriso`, `jq`, `ssh`, `curl`, a writable `/dev/kvm`
(the `kvm` group), and `docker` only when the image to test exists on the host. No root,
no libvirt. About 4 GB of RAM and 2 GB of disk for the VM, plus the 340 MB Debian image
cached in `~/.cache/maintenant-escalation-lab`.

A Pro licence key, as `MAINTENANT_LICENSE_KEY` in the environment or in
`compose.local.yml` at the repository root.

## Driving the bench

```sh
deploy/escalation-lab/lab up        # about 3 min on a cached image
deploy/escalation-lab/lab status
deploy/escalation-lab/lab run       # about 7 min
deploy/escalation-lab/lab down
```

`up` boots the machine, installs Docker through cloud-init, ships `LAB_IMAGE` when it
exists on the host (`docker save` over ssh) or lets the machine pull it, starts the stack,
checks the edition is Pro and that the escalation runner is enabled, then prints
`status`. `status` exits 0 only when the bench is green: machine up, product healthy,
Pro edition, the three apps running.

To test a branch build rather than the published image:

```sh
docker compose -f compose.local.yml build
LAB_IMAGE=ghcr.io/kolapsis/maintenant deploy/escalation-lab/lab up
```

`LAB_SSH_PORT`, `LAB_UI_PORT`, `LAB_MAIL_PORT` and `LAB_HOOK_PORT` move the host ports
(2222, 18080, 18025, 18081 by default, all on 127.0.0.1). `lab ssh` opens a shell in the
machine; `lab ssh 'cd stack && docker compose logs maintenant'` reads the product log.

## The scenario

`run` is replayable: it deletes the policy and channels of the previous run, empties the
mail catcher and uses fresh webhook sessions.

1. Four channels: `lab-l1-webhook`, `lab-l2-email` (oncall@lab.test), `lab-l3-email`
   (cto@lab.test), `lab-l3-webhook`. None is wired to a trigger: they only fire through
   the policy.
2. One policy, `lab: 3-level chain`, on critical alerts: level 1 at 60 s to the webhook,
   level 2 at 120 s to the on-call email, level 3 at 180 s to the CTO email and the pager
   webhook.
3. `docker kill --signal=SIGSEGV shop-api`, and `docker kill --signal=HUP shop-web` at the
   same time: nginx reloads and keeps running, so it must stay *running* with no alert. The signal to shop-api reaches the Go process directly
   (no `init: true`: tini never forwards SIGSEGV, and a PID 1 ignores it), the process
   dies with a crash code, which is what makes the container *down* rather than
   *completed* (exit 0, 137 or 143 would not alert).
4. The `container_down` alert fires once the container has been down 30 s, at the next
   30 s sweep: about a minute after the crash. Its severity is the container's alert
   severity, `warning` by default: the `maintenant.alert.severity: critical` label on
   `shop-api` is what makes the policy match.
5. The run starts with the alert. Runs are evaluated once a minute, so each level fires up
   to a minute after its delay: level 1 by T+2 min, level 2 and level 3 one evaluation
   apart, then the run ends `exhausted` and the last level's channels get the exhaustion
   notice.
6. `docker start shop-api`: the alert resolves on its own.

Every step is printed with its time; the timings in the report are counted from the
crash.

## The verdict

The report lands in `.state/runs/<timestamp>.json` and `verdict.jq` decides, from it:

- the alert is `container_down` and critical;
- the run ended `exhausted` after its third level;
- each level was delivered (`sent`) to its channels, and the exhaustion notice to both
  level 3 channels;
- the level 1 webhook received at least one request and the level 3 webhook at least
  two (the level and the exhaustion notice);
- oncall@lab.test received at least one mail and cto@lab.test at least two;
- every delivery carries its channel name;
- the alert resolved after the restart;
- shop-web is still `running` after its SIGHUP.

`run` exits 1 and lists what is missing when any of these fails. `bats deploy/escalation-lab/test`
checks the CLI contract and the verdict on fixtures without a machine.
