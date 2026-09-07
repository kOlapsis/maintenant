# cloud-init

`maintenant.yaml` provisions any Ubuntu cloud server with Docker and maintenant on first boot. Pass
it as user data when you create the server:

```bash
# Hetzner Cloud
hcloud server create \
  --name maintenant \
  --type cx22 \
  --image ubuntu-24.04 \
  --location nbg1 \
  --ssh-key my-key \
  --user-data-from-file deploy/cloud-init/maintenant.yaml

# DigitalOcean
doctl compute droplet create maintenant \
  --image ubuntu-24-04-x64 \
  --size s-2vcpu-4gb \
  --region fra1 \
  --ssh-keys my-key \
  --user-data-file deploy/cloud-init/maintenant.yaml \
  --wait

# Scaleway
scw instance server create \
  zone=fr-par-1 \
  type=PLAY2-NANO \
  image=ubuntu_noble \
  name=maintenant \
  ip=new \
  cloud-init=@deploy/cloud-init/maintenant.yaml

# OVHcloud Public Cloud (source your OpenStack RC file first)
openstack server create \
  --flavor d2-4 \
  --image "Ubuntu 24.04" \
  --key-name my-key \
  --user-data deploy/cloud-init/maintenant.yaml \
  maintenant

# Vultr (the CLI base64-encodes the file for you, do not do it yourself)
vultr-cli instance create \
  --region="ewr" \
  --plan="vc2-2c-4gb" \
  --os=2284 \
  --label="maintenant" \
  --userdata-file=deploy/cloud-init/maintenant.yaml
```

The dashboard is published on `127.0.0.1:8080` only. Reach it through an SSH tunnel, or front it
with a reverse proxy that authenticates.

Everything a single server does not cover — firewall rules, block volumes for the database, agent
enrolment over a private network, load balancer checks and managed Kubernetes — is per-provider:

- [Hetzner Cloud](https://docs.maintenant.dev/guides/hetzner/)
- [DigitalOcean](https://docs.maintenant.dev/guides/digitalocean/)
- [Scaleway](https://docs.maintenant.dev/guides/scaleway/)
- [OVHcloud](https://docs.maintenant.dev/guides/ovhcloud/)
- [Vultr](https://docs.maintenant.dev/guides/vultr/)
