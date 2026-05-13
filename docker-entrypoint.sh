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

# --keep-groups carries supplementary groups injected by compose `group_add`
# (e.g., the docker socket group) through to the unprivileged user.
exec setpriv --reuid=65534 --regid=65534 --keep-groups -- "$@"
