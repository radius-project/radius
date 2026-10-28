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


SCRIPT = Path(__file__).with_name("run-with-authz-logs.sh")
GATE = Path(__file__).with_name("authz-would-deny-check.sh")

MOCK = r'''
import json, os, pathlib, sys, time, urllib.parse
root = pathlib.Path(os.environ["MOCK_ROOT"])
args = sys.argv[1:]
if "--raw" not in args:
    assert args[args.index("--namespace") + 1] == "radius-system"
with (root / "pids").open("a") as f:
    f.write(str(os.getpid()) + "\n")
if (root / "hang").exists():
    time.sleep(60)
if args[0] == "get":
    if "--raw" not in args:
        if (root / "list-error").exists():
            sys.exit("inventory unavailable")
        if args[1] == "pod":
            assert "--ignore-not-found" in args
            if (root / "lookup-error").exists():
                sys.exit("pod lookup forbidden")
            name = args[2]
            pods = json.loads((root / "inventory").read_text())["items"]
            match = next((pod for pod in pods if pod["metadata"]["name"] == name), None)
            print(json.dumps(match) if match else "", flush=True)
            (root / (name + ".checked")).touch()
        else:
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
        with (root / (prefix + ".follows")).open("a") as f:
            f.write("follow\n")
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
                    if (root / "stream-error").exists():
                        sys.exit("stream transport failed")
                    break
                elif source.stat().st_ino != inode:
                    break
                elif (root / (prefix + ".disconnect")).exists():
                    (root / (prefix + ".disconnect")).unlink()
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
def replace_log(prefix, content):
    replacement = root / (prefix + ".replacement")
    replacement.write_text(content)
    os.replace(replacement, root / (prefix + ".log"))
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

    def run_collector(self, command="", expected=0, timeout=25,
                      bash="bash", cwd=None):
        result = subprocess.run(
            [bash, str(SCRIPT), sys.executable, "-c",
             COMMAND_HELPERS + command],
            env=self.env, cwd=cwd, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT, timeout=timeout,
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

    def test_command_arguments_are_not_reinterpreted_by_shell(self):
        argument = 'spaces; $(touch should-not-exist) "quotes"'
        result = subprocess.run(
            ["bash", str(SCRIPT), sys.executable, "-c",
             "import sys; print(sys.argv[1])", argument],
            env=self.env, text=True, stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT, timeout=25,
        )
        self.assertEqual(result.returncode, 0, result.stdout)
        self.assertIn(argument, result.stdout)
        self.assertTrue((self.output / "complete").is_file())

    def test_output_path_with_spaces_and_trailing_slash(self):
        self.output = self.root / "logs with spaces"
        self.env["AUTHZ_LOG_DIR"] = str(self.output) + "/"
        self.run_collector()
        self.assertTrue((self.output / "complete").is_file())

    def test_output_path_beginning_with_dash(self):
        for relative in ("-authz", "-parent/logs"):
            with self.subTest(path=relative):
                self.output = self.root / relative
                self.env["AUTHZ_LOG_DIR"] = relative
                self.run_collector(cwd=self.root)
                self.assertTrue((self.output / "complete").is_file())

    @unittest.skipUnless(sys.platform == "darwin", "macOS default Bash")
    def test_macos_default_bash_handles_empty_arrays(self):
        output = self.run_collector(
            '(root / "command-started").touch()\n', bash="/bin/bash")
        self.assertNotIn("unbound variable", output)
        self.assertTrue((self.root / "command-started").is_file())
        self.assertTrue((self.output / "complete").is_file())

    @unittest.skipUnless(sys.platform == "darwin", "macOS default Bash")
    def test_macos_default_bash_cleans_up_before_spawning(self):
        self.env["AUTHZ_LOG_DIR"] = ""
        output = self.run_collector(
            '(root / "command-started").touch()\n', expected=2,
            bash="/bin/bash")
        self.assertIn("AUTHZ_LOG_DIR must not be empty", output)
        self.assertNotIn("unbound variable", output)
        self.assertFalse((self.root / "command-started").exists())

    def test_kubectl_timeout_stops_collection_and_cleans_up(self):
        (self.root / "hang").touch()
        started = time.monotonic()
        result = self.run_collector(
            '(root / "command-started").touch()\n', expected=2, timeout=40)
        self.assertIn("kubectl timed out after 30 seconds", result)
        self.assertLess(time.monotonic() - started, 35)
        self.assertFalse((self.root / "command-started").exists())
        self.assertFalse((self.output / "complete").exists())

    def test_initialization_errors_never_start_command(self):
        for inventory in ("not json", '{"items": []}',
                          '{"items": [{}], "metadata": {}}'):
            with self.subTest(inventory=inventory):
                (self.root / "inventory").write_text(inventory)
                self.run_collector('(root / "command-started").touch()\n',
                                   expected=2)
                self.assertFalse((self.root / "command-started").exists())
                self.assertFalse((self.output / "complete").exists())
                self.output.rmdir()
                (self.root / "pids").unlink()

    def test_init_and_ephemeral_containers_are_collected(self):
        self.initial["status"]["initContainerStatuses"] = [{
            "name": "init", "restartCount": 0,
            "state": {"terminated": {"exitCode": 0}},
        }]
        self.initial["status"]["ephemeralContainerStatuses"] = [{
            "name": "debug", "restartCount": 0,
            "state": {"running": {}},
        }]
        self.inventory([self.initial])
        (self.root / "ucp-initial.init.log").write_text("init startup\n")
        (self.root / "ucp-initial.debug.log").write_text("debug startup\n")
        self.run_collector('wait_for("ucp-initial.debug.read")\n')
        self.assertIn("init startup", (
            self.output / "ucp-initial.ucp-initial-uid.init.0.log").read_text())
        self.assertIn("debug startup", (
            self.output / "ucp-initial.ucp-initial-uid.debug.0.log").read_text())

    def test_waiting_container_is_collected_when_it_starts(self):
        waiting = pod("ucp-initial")
        waiting["status"]["containerStatuses"][0]["state"] = {
            "waiting": {"reason": "ContainerCreating"}}
        self.inventory([waiting])
        command = f'''
wait_for("watch-started")
(root / "inventory").write_text(json.dumps({{
    "metadata": {{"resourceVersion": "124"}}, "items": [{self.initial!r}]}}))
event("MODIFIED", {self.initial!r})
wait_for("ucp-initial.ucp.read")
'''
        self.run_collector(command)
        self.assertIn("startup", (
            self.output / "ucp-initial.ucp-initial-uid.ucp.0.log").read_text())

    def test_pretty_printed_watch_events_are_parsed(self):
        command = f'''
wait_for("watch-started")
with (root / "events").open("a") as f:
    f.write(json.dumps({{"type": "MODIFIED", "object": {self.initial!r}}},
                       indent=2) + "\\n")
time.sleep(.3)
'''
        self.run_collector(command)
        self.assertTrue((self.output / "complete").is_file())

    def test_oversized_incomplete_watch_object_stops_command(self):
        command = '''
wait_for("watch-started")
(root / "command-pid").write_text(str(os.getpid()))
with (root / "events").open("a") as f:
    f.write('{"type":"MODIFIED","object":{"unfinished":"' + "x" * (17 * 1024 * 1024))
time.sleep(60)
'''
        started = time.monotonic()
        result = self.run_collector(command, expected=2, timeout=20)
        self.assertIn("pod watch object exceeded 16 MiB", result)
        self.assertLess(time.monotonic() - started, 15)
        self.assertFalse((self.output / "complete").exists())
        with self.assertRaises(ProcessLookupError):
            os.kill(int((self.root / "command-pid").read_text()), 0)

    def test_raw_watch_limit_counts_bytes_before_parsing(self):
        command = '''
wait_for("watch-started")
with (root / "events").open("a", encoding="utf-8") as f:
    f.write('{"unfinished":"' + "\\u00e9" * (9 * 1024 * 1024))
time.sleep(60)
'''
        result = self.run_collector(command, expected=2, timeout=20)
        self.assertIn("pod watch object exceeded 16 MiB", result)
        self.assertFalse((self.output / "complete").exists())

    def test_raw_watch_limit_resets_for_each_object(self):
        replacement = pod("ucp-replacement")
        replacement["metadata"]["annotations"] = {
            "escaped": 'braces } { [ ] and quotes " and backslash \\',
        }
        command = f'''
wait_for("watch-started")
(root / "ucp-replacement.ucp.log").write_text("replacement\\n")
with (root / "events").open("a") as f:
    for current in ({self.initial!r}, {replacement!r}):
        encoded = json.dumps({{"type": "ADDED", "object": current}})
        f.write(encoded[:-1] + " " * (9 * 1024 * 1024) + "}}\\n")
wait_for("ucp-replacement.ucp.read")
'''
        self.run_collector(command)
        self.assertTrue((self.output / "complete").is_file())

    def test_malformed_watch_data_fails_closed(self):
        for data in ('{"type": invalid}\n', '{"type": "MODIFIED"'):
            with self.subTest(data=data):
                (self.root / "events").write_text(data)
                result = self.run_collector("time.sleep(.5)\n", expected=2)
                self.assertIn("authz log collection failed", result)
                self.assertFalse((self.output / "complete").exists())
                for file in self.output.iterdir():
                    file.unlink()
                self.output.rmdir()
                (self.root / "pids").unlink()

    def test_final_snapshot_failure_never_marks_complete(self):
        command = '''
wait_for("ucp-initial.ucp.read")
(root / "log-error").touch()
'''
        self.run_collector(command, expected=2)
        self.assertFalse((self.output / "complete").exists())

    def test_wrapped_command_signal_status_is_preserved(self):
        command = '''
wait_for("ucp-initial.ucp.started")
import signal
os.kill(os.getpid(), signal.SIGTERM)
'''
        self.run_collector(command, expected=143)
        self.assertTrue((self.output / "complete").is_file())

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
wait_for("ucp-initial.ucp.read")
(root / "ucp-initial.ucp.previous.log").write_text("authzWouldDeny=true\\n")
restarted = {restarted!r}
(root / "inventory").write_text(json.dumps({{
    "metadata": {{"resourceVersion": "124"}}, "items": [restarted]}}))
replace_log("ucp-initial.ucp", "restarted\\n")
event("MODIFIED", restarted)
time.sleep(.3)
'''
        self.run_collector(command)
        self.assertTrue(any("authzWouldDeny=true" in f.read_text()
                            for f in self.output.glob("*.previous.log")))

    def test_stream_ends_before_restart_event(self):
        restarted = pod("ucp-initial", 1)
        command = f'''
wait_for("ucp-initial.ucp.read")
restarted = {restarted!r}
(root / "inventory").write_text(json.dumps({{
    "metadata": {{"resourceVersion": "124"}}, "items": [restarted]}}))
(root / "ucp-initial.ucp.previous.log").write_text("old-container\\n")
replace_log("ucp-initial.ucp", "new-container authzWouldDeny=true\\n")
wait_for("ucp-initial.checked")
event("MODIFIED", restarted)
wait_for("ucp-initial.ucp.denial-read")
'''
        self.run_collector(command)
        old = self.output / "ucp-initial.ucp-initial-uid.ucp.0.log"
        new = self.output / "ucp-initial.ucp-initial-uid.ucp.1.log"
        self.assertNotIn("new-container", old.read_text())
        self.assertIn("authzWouldDeny=true", new.read_text())
        self.assertEqual((self.root / "ucp-initial.ucp.follows").read_text().splitlines(),
                         ["follow", "follow"])
        self.assertTrue((self.output / "complete").is_file())

    def test_stream_ends_before_deletion_event(self):
        command = f'''
wait_for("ucp-initial.ucp.read")
(root / "inventory").write_text(json.dumps({{
    "metadata": {{"resourceVersion": "124"}}, "items": []}}))
(root / "ucp-initial.ucp.deleted").touch()
(root / "ucp-initial.ucp.log").unlink()
wait_for("ucp-initial.checked")
event("DELETED", {self.initial!r})
'''
        self.run_collector(command)
        self.assertEqual((self.root / "ucp-initial.ucp.follows").read_text().splitlines(),
                         ["follow"])
        self.assertTrue((self.output / "complete").is_file())

    def test_stream_ends_before_same_name_replacement_event(self):
        replacement = pod("ucp-initial")
        replacement["metadata"]["uid"] = "replacement-uid"
        command = f'''
wait_for("ucp-initial.ucp.read")
(root / "inventory").write_text(json.dumps({{
    "metadata": {{"resourceVersion": "124"}}, "items": [{replacement!r}]}}))
replace_log("ucp-initial.ucp", "replacement authzWouldDeny=true\\n")
wait_for("ucp-initial.checked")
event("DELETED", {self.initial!r})
event("ADDED", {replacement!r})
wait_for("ucp-initial.ucp.denial-read")
'''
        self.run_collector(command)
        old = self.output / "ucp-initial.ucp-initial-uid.ucp.0.log"
        new = self.output / "ucp-initial.replacement-uid.ucp.0.log"
        self.assertNotIn("replacement", old.read_text())
        self.assertIn("authzWouldDeny=true", new.read_text())
        self.assertEqual((self.root / "ucp-initial.ucp.follows").read_text().splitlines(),
                         ["follow", "follow"])

    def test_stream_ends_before_termination_event(self):
        terminated = pod("ucp-initial")
        terminated["status"]["containerStatuses"][0]["state"] = {
            "terminated": {"exitCode": 0}}
        command = f'''
wait_for("ucp-initial.ucp.read")
(root / "inventory").write_text(json.dumps({{
    "metadata": {{"resourceVersion": "124"}}, "items": [{terminated!r}]}}))
(root / "ucp-initial.ucp.disconnect").touch()
wait_for("ucp-initial.checked")
event("MODIFIED", {terminated!r})
'''
        self.run_collector(command)
        self.assertEqual((self.root / "ucp-initial.ucp.follows").read_text().splitlines(),
                         ["follow"])

    def test_reconnect_preserves_lines_at_same_timestamp(self):
        timestamp = "2026-10-08T00:00:00.000000001Z "
        (self.root / "ucp-initial.ucp.log").write_text(timestamp + "before\n")
        command = f'''
wait_for("ucp-initial.ucp.read")
(root / "ucp-initial.ucp.disconnect").touch()
wait_for("ucp-initial.checked")
with (root / "ucp-initial.ucp.log").open("a") as log:
    log.write({timestamp!r} + "authzWouldDeny=true\\n")
wait_for("ucp-initial.ucp.denial-read")
'''
        self.run_collector(command)
        logs = (self.output / "ucp-initial.ucp-initial-uid.ucp.0.log").read_text()
        self.assertIn(timestamp + "before\n", logs)
        self.assertIn(timestamp + "authzWouldDeny=true\n", logs)
        self.assertEqual((self.root / "ucp-initial.ucp.follows").read_text().splitlines(),
                         ["follow", "follow"])

    def test_reconnect_lookup_errors_fail_closed(self):
        command = '''
wait_for("ucp-initial.ucp.read")
(root / "lookup-error").touch()
(root / "ucp-initial.ucp.disconnect").touch()
time.sleep(.5)
'''
        result = self.run_collector(command, expected=2)
        self.assertIn("pod lookup forbidden", result)
        self.assertFalse((self.output / "complete").exists())

    def test_deleted_pod_does_not_hide_stream_failure(self):
        command = '''
wait_for("ucp-initial.ucp.read")
(root / "inventory").write_text(json.dumps({
    "metadata": {"resourceVersion": "124"}, "items": []}))
(root / "stream-error").touch()
(root / "ucp-initial.ucp.deleted").touch()
time.sleep(.5)
'''
        result = self.run_collector(command, expected=2)
        self.assertIn("log stream failed", result)
        self.assertFalse((self.output / "complete").exists())

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
import signal, subprocess, sys
wait_for("ucp-initial.ucp.started")
child = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(60)"])
def terminated(_signal, _frame):
    child.wait(timeout=5)
    raise SystemExit(143)
signal.signal(signal.SIGTERM, terminated)
(root / "child-pid").write_text(str(child.pid))
(root / "command-pid").write_text(str(os.getpid()))
time.sleep(60)
'''
        process = subprocess.Popen(
            ["bash", str(SCRIPT), sys.executable, "-c", command],
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
                    [(self.root / "command-pid").read_text(),
                     (self.root / "child-pid").read_text()])
            for pid in pids:
                with self.assertRaises(ProcessLookupError, msg="leaked process " + pid):
                    os.kill(int(pid), 0)
        finally:
            if process.poll() is None:
                process.terminate()
            process.communicate(timeout=10)


if __name__ == "__main__":
    unittest.main()
