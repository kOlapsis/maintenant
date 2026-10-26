"""Acceptance tests of 045-mcp-hardening on the example receiver (FR-040, US5).

Run from the repository root:
    python3 -I -m unittest discover -s examples/agent-webhook/__tests__ -p '*_test.py'
"""

import http.client
import importlib
import json
import os
import pathlib
import socket
import sys
import tempfile
import threading
import unittest
from http.server import ThreadingHTTPServer

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent.parent))

REPORTS = tempfile.mkdtemp(prefix="receiver-reports-")
os.environ["RECEIVER_TOKEN"] = ""
os.environ["RECEIVER_REPORTS"] = REPORTS
receiver = importlib.import_module("receiver")

VALID_ID = "0b7c3a4e-7d1f-4c62-9a55-3f3b8d0c2e11"


class ReceiverTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), receiver.Handler)
        threading.Thread(target=cls.server.serve_forever, daemon=True).start()
        cls.port = cls.server.server_address[1]

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()

    def setUp(self):
        while not receiver.jobs.empty():
            receiver.jobs.get_nowait()

    def post(self, body, headers=None):
        conn = http.client.HTTPConnection("127.0.0.1", self.port, timeout=5)
        conn.request("POST", "/", body=body, headers={"Content-Type": "application/json", **(headers or {})})
        resp = conn.getresponse()
        resp.read()
        conn.close()
        return resp.status

    def reports(self):
        return sorted(p.name for p in pathlib.Path(REPORTS).iterdir()) if pathlib.Path(REPORTS).exists() else []

    # US5 scenario 1, FR-040: an id that is not a UUID is refused before it can name a file.
    def test_non_uuid_alert_id_is_refused(self):
        for forged in [
            "../../etc/passwd",
            "../report",
            "",
            "abc",
            "0b7c3a4e7d1f4c629a553f3b8d0c2e11",
            "0b7c3a4e-7d1f-4c62-9a55-3f3b8d0c2e1",
            "0b7c3a4e-7d1f-4c62-9a55-3f3b8d0c2e11/x",
            "0b7c3a4e-7d1f-4c62-9a55-3f3b8d0c2e1g",
        ]:
            with self.subTest(id=forged):
                body = json.dumps({"event": "alert.fired", "alert": {"id": forged, "message": "boom"}})
                self.assertEqual(400, self.post(body))
                self.assertTrue(receiver.jobs.empty(), "a refused alert is never queued")
        self.assertEqual([], self.reports(), "no file is written")

    def test_alert_without_id_is_refused(self):
        self.assertEqual(400, self.post(json.dumps({"event": "alert.fired", "alert": {"message": "no id"}})))
        self.assertTrue(receiver.jobs.empty())

    # Contract: a well-formed alert is still queued. Guards a validation that would refuse every id.
    def test_uuid_alert_id_is_queued(self):
        body = json.dumps({"event": "alert.fired", "alert": {"id": VALID_ID, "message": "boom"}})
        self.assertEqual(202, self.post(body))
        self.assertEqual(VALID_ID, receiver.jobs.get(timeout=1)["id"])

    def test_other_events_are_logged_not_queued(self):
        self.assertEqual(202, self.post(json.dumps({"event": "alert.resolved", "alert": {"id": VALID_ID}})))
        self.assertTrue(receiver.jobs.empty())

    # US5 scenario 2, FR-040: a Content-Length above 1 MiB is refused without reading the body.
    def test_oversized_content_length_is_refused_without_reading(self):
        for declared in [1_048_577, 2_000_000]:
            with self.subTest(content_length=declared):
                conn = http.client.HTTPConnection("127.0.0.1", self.port, timeout=3)
                conn.putrequest("POST", "/")
                conn.putheader("Content-Type", "application/json")
                conn.putheader("Content-Length", str(declared))
                conn.endheaders()
                try:
                    resp = conn.getresponse()
                except socket.timeout:
                    self.fail("the receiver waited for the body instead of answering 413")
                self.assertEqual(413, resp.status)
                conn.close()
        self.assertTrue(receiver.jobs.empty())

    def test_body_at_the_limit_is_still_read(self):
        padding = "x" * (1_048_576 - len('{"event":"test","pad":""}'))
        body = '{"event":"test","pad":"' + padding + '"}'
        self.assertEqual(1_048_576, len(body))
        self.assertEqual(202, self.post(body))


if __name__ == "__main__":
    unittest.main()
