import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("portability", Path(__file__).with_name("demo.py"))
demo = importlib.util.module_from_spec(spec)
spec.loader.exec_module(demo)


class PeerEvidenceTests(unittest.TestCase):
    def test_same_payload_on_both_platforms(self):
        payload = '{"host":"server","port":8080,"ok":true}'
        kubernetes = "2026-10-27T20:42:38.066532Z " + payload
        aca = json.dumps({
            "TimeStamp": "2026-10-27T20:42:38.0665325+00:00",
            "Log": "F " + payload,
        })
        stamp_kubernetes, event_kubernetes = demo.peer_event(kubernetes, "kubernetes")
        stamp_aca, event_aca = demo.peer_event(aca, "aca")
        self.assertEqual(stamp_kubernetes, stamp_aca)
        self.assertEqual(event_kubernetes, event_aca)
        self.assertTrue(event_aca["ok"])

    def test_connection_failure_is_not_success(self):
        line = json.dumps({
            "TimeStamp": "2026-10-27T20:42:38+00:00",
            "Log": 'F {"port":80,"ok":false}',
        })
        _, record = demo.peer_event(line, "aca")
        self.assertFalse(record["ok"])

    def test_cli_connection_messages_are_not_traffic_evidence(self):
        line = json.dumps({
            "TimeStamp": "2026-10-27T20:42:38",
            "Log": "Successfully Connected to container",
        })
        with self.assertRaises(json.JSONDecodeError):
            demo.peer_event(line, "aca")


if __name__ == "__main__":
    unittest.main()
