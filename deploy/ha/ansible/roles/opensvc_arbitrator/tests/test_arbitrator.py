import http.client
import os
import subprocess
import sys
import threading
import unittest
import urllib.request
from unittest import mock

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "files"))
import arbitrator  # noqa: E402


def peer(name, role, disk, state="Connected"):
    return {
        "name": name,
        "connection-state": state,
        "peer-role": role,
        "peer_devices": [{"volume": 0, "peer-disk-state": disk}],
    }


def status(*connections):
    return [{"name": "maintenant", "role": "Secondary", "devices": [{"disk-state": "Diskless"}], "connections": list(connections)}]


PARTITION = status(peer("site-a", "Primary", "UpToDate"), peer("site-b", "Secondary", "Outdated"))
PRIMARY_LOST = status(peer("site-a", "Unknown", "DUnknown", "Connecting"), peer("site-b", "Secondary", "UpToDate"))
STEADY = status(peer("site-a", "Primary", "UpToDate"), peer("site-b", "Secondary", "UpToDate"))
RESYNC = status(peer("site-a", "Primary", "UpToDate"), peer("site-b", "Secondary", "Inconsistent"))


class Decide(unittest.TestCase):
    def test_partition_refuses_the_outdated_side(self):
        self.assertEqual(arbitrator.decide("site-b", PARTITION)[0], False)

    def test_partition_grants_the_primary(self):
        self.assertEqual(arbitrator.decide("site-a", PARTITION)[0], True)

    def test_primary_lost_grants_the_survivor(self):
        self.assertEqual(arbitrator.decide("site-b", PRIMARY_LOST)[0], True)

    def test_steady_state_grants_both(self):
        self.assertEqual([arbitrator.decide(n, STEADY)[0] for n in ("site-a", "site-b")], [True, True])

    def test_resync_refuses_the_inconsistent_side(self):
        self.assertEqual(arbitrator.decide("site-b", RESYNC)[0], False)

    def test_unknown_client_is_granted(self):
        self.assertEqual(arbitrator.decide(None, PARTITION)[0], True)

    def test_no_drbd_is_granted(self):
        self.assertEqual(arbitrator.decide("site-b", None)[0], True)

    def test_requester_outside_the_resource_is_granted(self):
        self.assertEqual(arbitrator.decide("site-c", PARTITION)[0], True)

    def test_failed_drbdsetup_reads_as_no_state(self):
        for failure in (FileNotFoundError, subprocess.TimeoutExpired("drbdsetup", 0.5), subprocess.CalledProcessError(10, "drbdsetup")):
            with mock.patch("subprocess.run", side_effect=failure):
                self.assertIsNone(arbitrator.drbd_status("maintenant"))


class Serve(unittest.TestCase):
    def setUp(self):
        self.server = arbitrator.serve({"port": 0, "path": "/arbitrator", "resource": "maintenant", "nodes": {"127.0.0.1": "site-b"}})
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.server.server_address[1]}/arbitrator"

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()

    def test_grant_answers_200(self):
        with mock.patch.object(arbitrator, "drbd_status", return_value=STEADY):
            self.assertEqual(urllib.request.urlopen(self.url, timeout=2).status, 200)

    def test_refusal_closes_without_an_answer(self):
        with mock.patch.object(arbitrator, "drbd_status", return_value=PARTITION):
            with self.assertRaises(http.client.RemoteDisconnected):
                urllib.request.urlopen(self.url, timeout=2)


if __name__ == "__main__":
    unittest.main()
