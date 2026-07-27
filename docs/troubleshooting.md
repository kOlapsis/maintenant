# Troubleshooting

---

## Permission denied on /var/run/docker.sock

**Symptom:** maintenant starts but shows no containers, and the logs contain:

```text
permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock
```

**Why it happens:** maintenant runs as `nobody` (uid 65534) by design — the Docker image never grants root access. The Docker socket on the host is owned by `root:docker`. The process needs membership in the socket's group, otherwise the kernel rejects the `open` call regardless of the read-only mount.

**Normally this is automatic.** The entrypoint detects the group of the mounted `/var/run/docker.sock` and grants the unprivileged user access to it — on plain Compose **and** Docker Swarm. Just mount the socket:

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    read_only: true
    security_opt:
      - no-new-privileges:true
    tmpfs:
      - /tmp:noexec,nosuid,size=64m
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - /proc:/host/proc:ro
      - maintenant-data:/data
    environment:
      MAINTENANT_ADDR: "0.0.0.0:8080"
      MAINTENANT_DB: "/data/maintenant.db"
    restart: unless-stopped
```

!!! warning "Don't rely on `group_add` for Swarm"
    `docker stack deploy` silently ignores `group_add` (it is not part of the Swarm service
    spec), which is why socket access used to fail on Swarm. Auto-detection makes `group_add`
    unnecessary there — see the [Swarm guide](guides/swarm.md).

**If it still fails** (non-standard socket path, socket proxy, or you want to pin the GID), set `DOCKER_GID` explicitly. Find the socket's group on the host:

```bash
stat -c '%g' /var/run/docker.sock
# or: getent group docker | cut -d: -f3
```

Create a `.env` file next to your `docker-compose.yml`:

```bash
DOCKER_GID=998   # replace with the number printed above
```

and pass it to the container (either as an environment variable, which the entrypoint reads, or via `group_add` on plain Compose):

```yaml
    environment:
      DOCKER_GID: "${DOCKER_GID}"
```

---

### SELinux (Fedora / RHEL / Rocky / CentOS)

If the GID fix above does not resolve the error, SELinux may be blocking the socket access. Check for recent denials:

```bash
ausearch -m AVC -ts recent
```

If you see a denial for `docker.sock`, add the `:z` relabel flag to the socket mount so SELinux applies the correct context:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro,z
```

The `:z` flag relabels the bind mount with a shared label (`svirt_sandbox_file_t`), which grants container processes access while keeping SELinux enforcing.

---

### Docker rootless

With rootless Docker, the socket is not at `/var/run/docker.sock` but at `$XDG_RUNTIME_DIR/docker.sock`, typically `/run/user/<uid>/docker.sock`. Adjust the bind mount accordingly:

```bash
# Find the socket path on the host
echo $XDG_RUNTIME_DIR/docker.sock
# e.g. /run/user/1000/docker.sock
```

```yaml
volumes:
  - /run/user/1000/docker.sock:/var/run/docker.sock:ro
```

Replace `1000` with the UID of the user running the rootless daemon (`id -u` on the host). The `group_add` configuration is not required in rootless mode — the socket is owned by the user, not by a `docker` group.

## An HTTPS host shows as down (or degraded) with "unknown authority"

The host is answering; maintenant just refuses to trust the certificate it presents. Since it is reachable, it is reported as **degraded** rather than down.

If the certificate comes from your own PKI, trust the root instead of disabling verification:

```yaml
environment:
  MAINTENANT_CA_CERT: /etc/maintenant/ca.pem
volumes:
  - ./ca.pem:/etc/maintenant/ca.pem:ro
```

Make sure the mounted file is readable by uid **65534** — the container runs unprivileged, and a root-owned `0600` file will not be read. If the endpoint is attached to an agent, set this on the **agent**: it is the one performing the probe.

Do not reach for `SSL_CERT_FILE`. Go treats it as a replacement for the whole system bundle rather than an addition, so setting it drops every public CA, and an unreadable file yields an empty trust store with no error reported.

As a last resort you can turn verification off for a single container endpoint with the label `maintenant.endpoint.http.tls-verify=false`, but that also stops expiry and hostname checks for it.
