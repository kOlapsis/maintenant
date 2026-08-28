# Hetzner Cloud

`cloud-init.yaml` provisions a Hetzner Cloud server with Docker and maintenant on first boot:

```bash
hcloud server create \
  --name maintenant \
  --type cx22 \
  --image ubuntu-24.04 \
  --location nbg1 \
  --ssh-key my-key \
  --user-data-from-file deploy/hetzner/cloud-init.yaml
```

The dashboard is published on `127.0.0.1:8080` only — reach it through an SSH tunnel, or front it
with a reverse proxy that authenticates.

Firewall rules, private-network agent enrolment, Hetzner Volumes for the database, Load Balancer
checks and Kubernetes on Hetzner are covered in the
[Hetzner Cloud Deployment Guide](https://docs.maintenant.dev/guides/hetzner/).
