# Troubleshooting

---

## Permission denied on /var/run/docker.sock

**Symptom:** maintenant starts but shows no containers, and the logs contain:

```text
permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock
```

**Why it happens:** maintenant runs as `nobody` (uid 65534) by design — the Docker image never grants root access. The Docker socket on the host is owned by `root:docker`. Without `group_add`, the `nobody` user has no group membership that grants access to the socket, so the kernel rejects the `open` call regardless of the read-only mount.

**Fix:** find the Docker group GID on the host and pass it to the container via `group_add`.

```bash
# On the host
getent group docker | cut -d: -f3
```

Create a `.env` file next to your `docker-compose.yml`:

```bash
DOCKER_GID=998   # replace with the number printed above
```

Your `docker-compose.yml` must include `group_add`:

```yaml
services:
  maintenant:
    image: ghcr.io/kolapsis/maintenant:latest
    read_only: true
    security_opt:
      - no-new-privileges:true
    group_add:
      - "${DOCKER_GID:-983}"
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

Then restart:

```bash
docker compose up -d
```

The fallback `983` in the template is a common value but not universal — it varies by distribution, Docker install method, and host configuration. Always set `DOCKER_GID` explicitly for production deployments.

!!! tip "Alternative: stat instead of getent"
    `stat -c '%g' /var/run/docker.sock` also prints the socket's group GID and works on systems without `getent`.

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
