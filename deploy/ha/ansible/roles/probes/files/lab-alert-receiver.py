#!/usr/bin/env python3
"""Append each alert POSTed by the cluster nodes as one line of an ndjson journal."""

import json
import sys
import time
from http.server import BaseHTTPRequestHandler, HTTPServer


class Receiver(BaseHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length") or 0))
        try:
            alert = json.loads(body)
        except ValueError:
            alert = body.decode("utf-8", "replace")
        with open(self.server.journal, "a", encoding="utf-8") as journal:
            journal.write(json.dumps({"received_at": time.time(), "alert": alert}) + "\n")
        self.send_response(204)
        self.end_headers()

    def log_message(self, *args):
        pass


def main():
    port, journal = int(sys.argv[1]), sys.argv[2]
    server = HTTPServer(("", port), Receiver)
    server.journal = journal
    server.serve_forever()


if __name__ == "__main__":
    main()
