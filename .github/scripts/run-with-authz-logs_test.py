#!/usr/bin/env python3
# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

"""Process-level tests of the workflow-owned collector; no cluster required."""

import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest


SCRIPT = Path(__file__).with_name("run-with-authz-logs.py")
GATE = Path(__file__).with_name("authz-would-deny-check.sh")

MOCK = r'''
import json, os, pathlib, sys, time, urllib.parse
root = pathlib.Path(os.environ["MOCK_ROOT"])
args = sys.argv[1:]
if "--raw" not in args:
    assert args[args.index("--namespace") + 1] == "radius-system"
with (root / "pids").open("a") as f:
    f.write(str(os.getpid()) + "\n")
if args[0] == "get":
    if "--raw" not in args:
        if (root / "list-error").exists():
            sys.exit("inventory unavailable")
        print((root / "inventory").read_text())
    else:
        url = urllib.parse.urlparse(args[args.index("--raw") + 1])
        assert url.path == "/api/v1/namespaces/radius-system/pods"
        assert urllib.parse.parse_qs(url.query)["resourceVersion"] == ["123"]
        if (root / "watch-error").exists():
            sys.exit("watch unavailable")
        (root / "watch-started").touch()
        with (root / "events").open() as f:
            while True:
                line = f.readline()
                if line:
                    print(line, end="", flush=True)
                else:
                    time.sleep(.02)
elif args[0] == "logs":
    pod = args[1]
    container = args[args.index("--container") + 1]
    prefix = pod + "." + container
    if (root / "log-error").exists():
        sys.exit("logs unavailable")
    previous = "--previous" in args
    if previous:
        prefix += ".previous"
    source = root / (prefix + ".log")
    if "--follow" not in args:
        print(source.read_text(), end="", flush=True)
    else:
        if (root / "follow-error").exists():
            sys.exit("follow unavailable")
        (root / (prefix + ".started")).touch()
        with source.open() as f:
            inode = os.fstat(f.fileno()).st_ino
            while True:
                line = f.readline()
                if line:
                    print(line, end="", flush=True)
                    (root / (prefix + ".read")).touch()
                    if "authzWouldDeny=true" in line:
                        (root / (prefix + ".denial-read")).touch()
                elif (root / (prefix + ".deleted")).exists():
                    break
                elif source.stat().st_ino != inode:
                    break
                else:
                    time.sleep(.02)
else:
    sys.exit("unexpected kubectl invocation")
'''

COMMAND_HELPERS = r'''
import json, os, pathlib, time
root = pathlib.Path(os.environ["MOCK_ROOT"])
def wait_for(name):
    deadline = time.monotonic() + 10
    while not (root / name).exists():
        if time.monotonic() > deadline:
            raise RuntimeError("timed out waiting for " + name)
        time.sleep(.02)
def event(kind, pod):
    with (root / "events").open("a") as f:
        f.write(json.dumps({"type": kind, "object": pod}) + "\n")
'''


def pod(name, restarts=0):
    return {
        "metadata": {"name": name, "uid": name + "-uid"},
        "status": {"containerStatuses": [{
            "name": "ucp", "restartCount": restarts,
            "state": {"running": {"startedAt": "2026-10-08T00:00:00Z"}},
        }]},
    }


class CollectorTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.output = self.root / "collected"
        self.bin = self.root / "bin"
        self.bin.mkdir()
        kubectl = self.bin / "kubectl"
        kubectl.write_text("#!" + sys.executable + "\n" + MOCK)
        kubectl.chmod(0o755)
        self.env = dict(os.environ, MOCK_ROOT=str(self.root),
                        AUTHZ_LOG_DIR=str(self.output),
                        PATH=str(self.bin) + os.pathsep + os.environ["PATH"])
        self.initial = pod("ucp-initial")
        self.inventory([self.initial])
        (self.root / "events").touch()
        (self.root / "ucp-initial.ucp.log").write_text("startup\n")

    def inventory(self, pods):
        (self.root / "inventory").write_text(json.dumps({
            "metadata": {"resourceVersion": "123"}, "items": pods,
        }))

    def run_collector(self, command="", expected=0):
        result = subprocess.run(
            [sys.executable, str(SCRIPT), sys.executable, "-c",
             COMMAND_HELPERS + command],
            env=self.env, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT, timeout=25,
        )
        self.assertEqual(result.returncode, expected, result.stdout)
        if (self.root / "pids").exists():
            for pid in (self.root / "pids").read_text().splitlines():
                with self.assertRaises(ProcessLookupError, msg="leaked kubectl " + pid):
                    os.kill(int(pid), 0)
        return result.stdout

    def test_clean_run_and_cleanup(self):
        self.run_collector('wait_for("ucp-initial.ucp.started")\n')
        self.assertTrue((self.output / "complete").is_file())
        self.assertIn("startup", "".join(f.read_text() for f in self.output.glob("*.log")))

    def test_replacement_after_first_test_finishes_is_gated(self):
        replacement = pod("ucp-replacement")
        command = f'''
wait_for("ucp-initial.ucp.started")
wait_for("watch-started")
# The first test has finished, but the enclosing functional command is still running.
(root / "first-test-finished").touch()
replacement = {replacement!r}
(root / "ucp-replacement.ucp.log").write_text('startup\\n')
event("ADDED", replacement)
wait_for("ucp-replacement.ucp.read")
with (root / "ucp-replacement.ucp.log").open("a") as log:
    log.write("authzWouldDeny=true\\n")
wait_for("ucp-replacement.ucp.denial-read")
event("DELETED", replacement)
(root / "ucp-replacement.ucp.deleted").touch()
time.sleep(.3)
'''
        self.run_collector(command)
        logs = "".join(f.read_text() for f in self.output.glob("*.log"))
        self.assertIn("authzWouldDeny=true", logs)
        result = subprocess.run(["bash", str(GATE), "--logs-dir", str(self.output)],
                                text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        self.assertEqual(result.returncode, 1, result.stdout)
        self.assertIn("ucp-replacement", result.stdout)

    def test_restarts_capture_previous_and_new_logs(self):
        restarted = pod("ucp-initial", 1)
        command = f'''
wait_for("ucp-initial.ucp.started")
(root / "ucp-initial.ucp.log").rename(root / "old-container.log")
(root / "ucp-initial.ucp.log").write_text("restarted\\n")
(root / "ucp-initial.ucp.previous.log").write_text("authzWouldDeny=true\\n")
restarted = {restarted!r}
(root / "inventory").write_text(json.dumps({{
    "metadata": {{"resourceVersion": "124"}}, "items": [restarted]}}))
event("MODIFIED", restarted)
time.sleep(.3)
'''
        self.run_collector(command)
        self.assertTrue(any("authzWouldDeny=true" in f.read_text()
                            for f in self.output.glob("*.previous.log")))

    def test_test_failure_is_preserved_and_logs_are_completed(self):
        self.run_collector('wait_for("ucp-initial.ucp.started")\nraise SystemExit(7)\n',
                           expected=7)
        self.assertTrue((self.output / "complete").is_file())

    def test_collection_failures_never_mark_complete(self):
        for failure in ("list-error", "watch-error", "log-error", "follow-error"):
            with self.subTest(failure=failure):
                (self.root / failure).touch()
                result = self.run_collector("time.sleep(.5)\n", expected=2)
                self.assertIn("authz log collection failed", result)
                self.assertFalse((self.output / "complete").exists())
                (self.root / failure).unlink()
                # Each invocation requires a fresh output directory.
                if self.output.exists():
                    for file in self.output.iterdir():
                        file.unlink()
                    self.output.rmdir()
                (self.root / "pids").unlink(missing_ok=True)

    def test_stale_output_is_rejected(self):
        self.output.mkdir()
        (self.output / "complete").touch()
        self.run_collector(expected=2)

    def test_watch_error_event_fails(self):
        command = '''
wait_for("watch-started")
event("ERROR", {"message": "resource version expired"})
time.sleep(.5)
'''
        self.run_collector(command, expected=2)
        self.assertFalse((self.output / "complete").exists())

    def test_cancellation_stops_command_and_collectors(self):
        command = COMMAND_HELPERS + '''
wait_for("ucp-initial.ucp.started")
(root / "command-pid").write_text(str(os.getpid()))
time.sleep(60)
'''
        process = subprocess.Popen(
            [sys.executable, str(SCRIPT), sys.executable, "-c", command],
            env=self.env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
            text=True,
        )
        try:
            deadline = time.monotonic() + 10
            while not (self.root / "command-pid").exists():
                if time.monotonic() > deadline or process.poll() is not None:
                    self.fail("wrapped command did not start")
                time.sleep(.02)
            process.send_signal(signal.SIGTERM)
            output, _ = process.communicate(timeout=10)
            self.assertEqual(process.returncode, 143, output)
            self.assertFalse((self.output / "complete").exists())
            pids = ((self.root / "pids").read_text().splitlines() +
                    [(self.root / "command-pid").read_text()])
            for pid in pids:
                with self.assertRaises(ProcessLookupError, msg="leaked process " + pid):
                    os.kill(int(pid), 0)
        finally:
            if process.poll() is None:
                process.terminate()
            process.communicate(timeout=10)


if __name__ == "__main__":
    unittest.main()
