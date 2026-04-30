#!/bin/sh
set -e

# Bind-mounted /data/shm arrives with host ownership; fix it so the
# unprivileged runtime user can write its SHM identity file.
mkdir -p /data/shm
chown 65534:65534 /data/shm

# --keep-groups carries supplementary groups injected by compose `group_add`
# (e.g., the docker socket group) through to the unprivileged user.
exec setpriv --reuid=65534 --regid=65534 --keep-groups -- "$@"
