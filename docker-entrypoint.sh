#!/bin/sh
set -e

# If the first arg looks like a flag, assume it's flags for the maintenant
# binary and prepend it. Matches the idiom used by official postgres/redis
# images so install templates can pass `--mode=agent ...` directly.
if [ "${1#-}" != "$1" ]; then
    set -- /app/maintenant "$@"
fi

# Ensure the data directory used by the chosen mode is writable by the
# unprivileged runtime user. Covers both named volumes (Docker creates them
# root:root when the target path doesn't exist in the image) and bind mounts
# (host ownership leaks into the container).
case " $* " in
    *" --mode=agent "*|*" --mode agent "*)
        mkdir -p /var/lib/maintenant
        chown 65534:65534 /var/lib/maintenant
        ;;
    *)
        mkdir -p /data/shm
        chown 65534:65534 /data/shm
        ;;
esac

# Auto-detect the mounted Docker socket's group so socket access works without
# `group_add` (which `docker stack deploy` silently drops on Swarm — see #29).
SUPP=""
add_gid() {
    [ -n "$1" ] || return 0
    case ",$SUPP," in *",$1,"*) ;; *) SUPP="${SUPP:+$SUPP,}$1" ;; esac
}
# Detected and inherited groups never carry gid 0: root's group would be a silent
# privilege upgrade. Only an explicit DOCKER_GID=0 grants it, for hosts whose
# socket is root:root with no docker group (Synology DSM Container Manager).
add_group() {
    [ "$1" != "0" ] || return 0
    add_gid "$1"
}

root_sock=""
for sock in /var/run/docker.sock /run/docker.sock; do
    [ -S "$sock" ] || continue
    gid="$(stat -c '%g' "$sock" 2>/dev/null)"
    if [ "$gid" = "0" ]; then
        root_sock="$sock"
    fi
    add_group "$gid"
done

if [ -n "${DOCKER_GID:-}" ]; then
    add_gid "$DOCKER_GID"
elif [ -n "$root_sock" ]; then
    echo "maintenant: $root_sock belongs to group root (gid 0) and the runtime drops to an unprivileged user, so the Docker API will be unreachable. Set DOCKER_GID=0 to grant that group explicitly, or front the socket with a docker-socket-proxy (https://docs.maintenant.dev/security/#recommended-docker-socket-proxy)." >&2
fi

for g in $(id -G 2>/dev/null); do add_group "$g"; done

if [ -n "$SUPP" ]; then
    exec setpriv --reuid=65534 --regid=65534 --groups="$SUPP" -- "$@"
else
    exec setpriv --reuid=65534 --regid=65534 --clear-groups -- "$@"
fi
