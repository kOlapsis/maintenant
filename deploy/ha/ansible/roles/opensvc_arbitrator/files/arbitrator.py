#!/usr/bin/env python3
import json
import subprocess
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

DRBD_TIMEOUT_S = 0.5


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
        try:
            grant, reason = decide(node, drbd_status(config["resource"]) if node else None)
        except (AttributeError, IndexError, KeyError, TypeError):
            grant, reason = True, "unreadable drbd state"
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
