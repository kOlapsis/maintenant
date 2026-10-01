#!/usr/bin/env python3
"""Receive maintenant alert webhooks and hand each fired alert to Claude Code."""

import hmac
import json
import os
import pathlib
import queue
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HERE = pathlib.Path(__file__).resolve().parent
LISTEN = os.environ.get("RECEIVER_LISTEN", "127.0.0.1:9099")
TOKEN = os.environ.get("RECEIVER_TOKEN", "")
MCP_CONFIG = os.environ.get("RECEIVER_MCP_CONFIG", str(HERE / "mcp.json"))
REPORTS = pathlib.Path(os.environ.get("RECEIVER_REPORTS", HERE / "reports"))
TIMEOUT = int(os.environ.get("RECEIVER_TIMEOUT", "600"))

READ_TOOLS = [
    "list_alerts",
    "list_agents",
    "list_containers",
    "get_container",
    "get_container_logs",
    "get_resources",
    "get_top_consumers",
    "list_endpoints",
    "get_endpoint_history",
    "list_heartbeats",
    "list_certificates",
    "get_updates",
    "get_security_insights",
    "get_health",
]

PROMPT = """maintenant just fired this alert:

{alert}

Investigate it with the maintenant MCP tools, read-only. Start from the entity
in the alert, read its logs and recent history, check the other active alerts
on the same agent, and look for what changed shortly before fired_at.

Answer in under 15 lines: what is failing, the most likely cause, the evidence
you saw (quote log lines), and the fix you would try first.
"""

jobs = queue.Queue()


def investigate(alert):
    REPORTS.mkdir(parents=True, exist_ok=True)
    cmd = [
        "claude", "-p", PROMPT.format(alert=json.dumps(alert, indent=2)),
        "--setting-sources", "project",
        "--mcp-config", MCP_CONFIG,
        "--strict-mcp-config",
        "--tools", "",
        "--allowedTools", *[f"mcp__maintenant__{t}" for t in READ_TOOLS],
    ]
    try:
        out = subprocess.run(cmd, cwd=REPORTS, capture_output=True, text=True, timeout=TIMEOUT)
        report = out.stdout if out.returncode == 0 else f"claude exited {out.returncode}\n{out.stderr}"
    except subprocess.TimeoutExpired:
        report = f"investigation timed out after {TIMEOUT}s"
    path = REPORTS / f"{alert['id']}.md"
    path.write_text(f"# {alert.get('message', alert['id'])}\n\n{report}\n")
    print(f"report written to {path}", flush=True)


def worker():
    while True:
        alert = jobs.get()
        try:
            investigate(alert)
        except Exception as exc:
            print(f"investigation of {alert.get('id')} failed: {exc}", file=sys.stderr, flush=True)
        jobs.task_done()


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if TOKEN and not hmac.compare_digest(self.headers.get("Authorization", ""), f"Bearer {TOKEN}"):
            self.send_response(401)
            self.end_headers()
            return
        try:
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))))
        except ValueError:
            self.send_response(400)
            self.end_headers()
            return
        self.send_response(202)
        self.end_headers()
        event, alert = body.get("event"), body.get("alert") or {}
        print(f"{event} {alert.get('severity', '')} {alert.get('message', '')}", flush=True)
        if event == "alert.fired":
            jobs.put(alert)


if __name__ == "__main__":
    if not TOKEN:
        sys.exit("set RECEIVER_TOKEN, and send it from the channel as 'Authorization: Bearer <token>'")
    threading.Thread(target=worker, daemon=True).start()
    host, port = LISTEN.rsplit(":", 1)
    print(f"listening on {LISTEN}", flush=True)
    ThreadingHTTPServer((host, int(port)), Handler).serve_forever()
