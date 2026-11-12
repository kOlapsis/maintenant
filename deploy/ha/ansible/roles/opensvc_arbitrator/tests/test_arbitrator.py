import http.client
import os
import subprocess
import sys
import threading
import time
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

PG_PARTITION = {"site-a": (False, ""), "site-b": (True, "")}
PG_PRIMARY_LOST = {"site-a": None, "site-b": (True, "")}
PG_STEADY = {"site-a": (False, ""), "site-b": (True, "streaming")}
PG_NO_PRIMARY = {"site-a": (True, ""), "site-b": (True, "")}
PG = {"port": 5432, "database": "postgres", "user": "maintenant_arbitrator", "passfile": "/etc/opensvc-arbitrator.pgpass"}
NODES = {"10.10.9.11": "site-a", "10.10.9.12": "site-b"}


def fake_psql(answers, delay=0.0):
    def run(args, **kwargs):
        time.sleep(delay)
        answer = next(a for host, a in answers.items() if f"host={host} " in args[args.index("-d") + 1])
        if isinstance(answer, BaseException):
            raise answer
        return subprocess.CompletedProcess(args, 0, stdout=answer, stderr="")

    return run


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


class DecidePostgres(unittest.TestCase):
    def test_partition_refuses_the_standby_that_lost_the_stream(self):
        for receiver in ("", "waiting", "starting"):
            states = dict(PG_PARTITION, **{"site-b": (True, receiver)})
            self.assertEqual(arbitrator.decide_postgres("site-b", states)[0], False)

    def test_partition_grants_the_primary(self):
        self.assertEqual(arbitrator.decide_postgres("site-a", PG_PARTITION)[0], True)

    def test_primary_lost_grants_the_standby(self):
        self.assertEqual(arbitrator.decide_postgres("site-b", PG_PRIMARY_LOST)[0], True)

    def test_steady_state_grants_both(self):
        self.assertEqual([arbitrator.decide_postgres(n, PG_STEADY)[0] for n in ("site-a", "site-b")], [True, True])

    def test_no_primary_grants_the_standby(self):
        self.assertEqual(arbitrator.decide_postgres("site-b", PG_NO_PRIMARY)[0], True)

    def test_silent_requester_is_refused_while_a_primary_answers(self):
        self.assertEqual(arbitrator.decide_postgres("site-b", {"site-a": (False, ""), "site-b": None})[0], False)

    def test_silent_requester_is_granted_when_no_primary_answers(self):
        self.assertEqual(arbitrator.decide_postgres("site-b", {"site-a": None, "site-b": None})[0], True)

    def test_unknown_client_is_granted(self):
        self.assertEqual(arbitrator.decide_postgres(None, PG_PARTITION)[0], True)


class PgState(unittest.TestCase):
    def test_reads_recovery_and_receiver(self):
        for out, state in (("t|streaming\n", (True, "streaming")), ("t|\n", (True, "")), ("f|\n", (False, ""))):
            with mock.patch("subprocess.run", side_effect=fake_psql({"10.10.9.12": out})):
                self.assertEqual(arbitrator.pg_state("10.10.9.12", PG), state)

    def test_unreadable_output_reads_as_no_state(self):
        for out in ("", "\n", "t\n", "x|streaming\n", "t|streaming|x\n", "t|streaming\nf|\n", "t|Streaming\n", " t|streaming\n"):
            with mock.patch("subprocess.run", side_effect=fake_psql({"10.10.9.12": out})):
                self.assertIsNone(arbitrator.pg_state("10.10.9.12", PG), out)

    def test_failed_psql_reads_as_no_state(self):
        for failure in (FileNotFoundError(), subprocess.TimeoutExpired("psql", 0.5), subprocess.CalledProcessError(2, "psql")):
            with mock.patch("subprocess.run", side_effect=fake_psql({"10.10.9.12": failure})):
                self.assertIsNone(arbitrator.pg_state("10.10.9.12", PG))

    def test_psql_is_capped_and_never_prompts(self):
        with mock.patch("subprocess.run", side_effect=fake_psql({"10.10.9.12": "f|\n"})) as run:
            arbitrator.pg_state("10.10.9.12", PG)
        args, kwargs = run.call_args
        self.assertEqual(kwargs["timeout"], arbitrator.PSQL_TIMEOUT_S)
        self.assertEqual(args[0][:5], ["psql", "-X", "-A", "-t", "-w"])
        self.assertIn("connect_timeout=1", args[0][6])

    def test_nodes_are_queried_in_parallel(self):
        answers = {"10.10.9.11": "f|\n", "10.10.9.12": "t|\n"}
        with mock.patch("subprocess.run", side_effect=fake_psql(answers, delay=0.4)):
            start = time.monotonic()
            states = arbitrator.pg_states(NODES, PG)
        self.assertLess(time.monotonic() - start, 0.7)
        self.assertEqual(states, PG_PARTITION)

    def test_a_node_past_the_cap_reads_as_unreachable(self):
        answers = {"10.10.9.11": subprocess.TimeoutExpired("psql", arbitrator.PSQL_TIMEOUT_S), "10.10.9.12": "t|\n"}
        with mock.patch("subprocess.run", side_effect=fake_psql(answers, delay=arbitrator.PSQL_TIMEOUT_S)):
            start = time.monotonic()
            states = arbitrator.pg_states(NODES, PG)
        self.assertLess(time.monotonic() - start, 1)
        self.assertEqual(states, PG_PRIMARY_LOST)
        self.assertEqual(arbitrator.decide_postgres("site-b", states)[0], True)


class Serve(unittest.TestCase):
    def start(self, backend):
        config = {"port": 0, "path": "/arbitrator", "backend": backend, "resource": "maintenant", "postgres": PG, "nodes": {"127.0.0.1": "site-b"}}
        server = arbitrator.serve(config)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return f"http://127.0.0.1:{server.server_address[1]}/arbitrator"

    def test_grant_answers_200(self):
        url = self.start("drbd")
        with mock.patch.object(arbitrator, "drbd_status", return_value=STEADY):
            self.assertEqual(urllib.request.urlopen(url, timeout=2).status, 200)

    def test_refusal_closes_without_an_answer(self):
        url = self.start("drbd")
        with mock.patch.object(arbitrator, "drbd_status", return_value=PARTITION):
            with self.assertRaises(http.client.RemoteDisconnected):
                urllib.request.urlopen(url, timeout=2)

    def test_postgres_refusal_closes_without_an_answer(self):
        url = self.start("postgres")
        with mock.patch.object(arbitrator, "pg_states", return_value=PG_PARTITION):
            with self.assertRaises(http.client.RemoteDisconnected):
                urllib.request.urlopen(url, timeout=2)

    def test_postgres_grant_answers_200(self):
        url = self.start("postgres")
        with mock.patch.object(arbitrator, "pg_states", return_value=PG_STEADY):
            self.assertEqual(urllib.request.urlopen(url, timeout=2).status, 200)

    def test_no_backend_grants_every_vote(self):
        url = self.start("none")
        with mock.patch.object(arbitrator, "drbd_status", return_value=PARTITION):
            self.assertEqual(urllib.request.urlopen(url, timeout=2).status, 200)


if __name__ == "__main__":
    unittest.main()
