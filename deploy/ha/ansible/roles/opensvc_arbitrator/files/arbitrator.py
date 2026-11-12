#!/usr/bin/env python3
import json
import re
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

DRBD_TIMEOUT_S = 0.5
PSQL_TIMEOUT_S = 0.5
PSQL_QUERY = "SELECT pg_is_in_recovery(), coalesce((SELECT status FROM pg_stat_wal_receiver), '')"
PSQL_OUTPUT = re.compile(r"([tf])\|([a-z]*)\n?")


def disks(connection):
    return {d.get("peer-disk-state") for d in connection.get("peer_devices", [])}


def decide(requester, status):
    """Return (grant, reason) for the node named requester, given the output of drbdsetup status --json or None."""
    if requester is None:
        return True, "unknown client"
    if not status:
        return True, "no drbd state"
    connections = {c.get("name"): c for c in status[0].get("connections", [])}
    if requester not in connections:
        return True, "not a drbd peer"
    mine = disks(connections[requester])
    if mine == {"UpToDate"}:
        return True, "disk UpToDate"
    state = ",".join(sorted(map(str, mine))) or "no disk"
    for name, c in connections.items():
        if (
            name != requester
            and c.get("connection-state") == "Connected"
            and c.get("peer-role") == "Primary"
            and disks(c) == {"UpToDate"}
        ):
            return False, f"disk {state}, {name} is the connected UpToDate primary"
    return True, f"disk {state}, no connected UpToDate primary"


def decide_postgres(requester, states):
    """Return (grant, reason) for the node named requester, given each node's (in_recovery, receiver_status), None for a node that did not answer."""
    if requester is None:
        return True, "unknown client"
    mine = states.get(requester)
    if mine is not None:
        recovery, receiver = mine
        if not recovery:
            return True, "primary"
        if receiver == "streaming":
            return True, "standby streaming"
        what = f"standby receiver {receiver or 'absent'}"
    else:
        what = "no postgres state"
    for name, state in sorted(states.items()):
        if name != requester and state is not None and not state[0]:
            return False, f"{what}, {name} is a reachable primary"
    return True, f"{what}, no reachable primary"


def pg_state(address, pg):
    conninfo = (
        f"host={address} port={pg['port']} dbname={pg['database']} user={pg['user']} "
        f"passfile={pg['passfile']} connect_timeout=1"
    )
    try:
        out = subprocess.run(
            ["psql", "-X", "-A", "-t", "-w", "-d", conninfo, "-c", PSQL_QUERY],
            capture_output=True,
            text=True,
            timeout=PSQL_TIMEOUT_S,
            check=True,
        ).stdout
    except (OSError, subprocess.SubprocessError, UnicodeDecodeError):
        return None
    m = PSQL_OUTPUT.fullmatch(out)
    return (m[1] == "t", m[2]) if m else None


def pg_states(nodes, pg):
    with ThreadPoolExecutor(len(nodes)) as pool:
        return dict(zip(nodes.values(), pool.map(lambda address: pg_state(address, pg), nodes)))


def drbd_status(resource):
    try:
        out = subprocess.run(
            ["drbdsetup", "status", "--json", resource],
            capture_output=True,
            timeout=DRBD_TIMEOUT_S,
            check=True,
        ).stdout
        return json.loads(out)
    except (OSError, subprocess.SubprocessError, ValueError):
        return None


class Handler(BaseHTTPRequestHandler):
    timeout = 5

    def do_GET(self):
        config = self.server.config
        if self.path.partition("?")[0] != config["path"]:
            self.send_error(404)
            return
        ip = self.client_address[0]
        node = config["nodes"].get(ip)
        backend = config["backend"]
        try:
            if backend == "drbd":
                grant, reason = decide(node, drbd_status(config["resource"]) if node else None)
            elif backend == "postgres":
                grant, reason = decide_postgres(node, pg_states(config["nodes"], config["postgres"]) if node else {})
            else:
                grant, reason = True, f"no rule for backend {backend}"
        except (AttributeError, IndexError, KeyError, TypeError):
            grant, reason = True, f"unreadable {backend} state"
        print(f"{ip} {node or '-'} {'grant' if grant else 'refuse'}: {reason}", flush=True)
        if not grant:
            # om3 rc47 counts any HTTP answer as a vote, a 403 included: only a closed connection withholds it.
            self.close_connection = True
            return
        body = b"arbitrator\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


def serve(config):
    server = ThreadingHTTPServer(("", config["port"]), Handler)
    server.config = config
    return server


def main():
    with open(sys.argv[1], encoding="utf-8") as f:
        config = json.load(f)
    serve(config).serve_forever()


if __name__ == "__main__":
    main()
