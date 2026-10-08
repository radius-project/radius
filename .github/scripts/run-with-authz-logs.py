#!/usr/bin/env python3
# Copyright 2026 The Radius Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

"""Run a command with workflow-owned radius-system log collection.

Requires Python 3.9+ and kubectl. AUTHZ_LOG_DIR must be a new directory.
List/watch resource versions avoid a polling gap for replacement pods. Each
container incarnation has a separate file, including the last previous restart.
Collection errors fail closed. An interrupted watch is an error, not a silent
reconnect that could miss deleted pods. The command's nonzero status is preserved
when collection succeeds. All owned process groups are stopped before returning.
"""

import json
import os
from pathlib import Path
import queue
import signal
import subprocess
import sys
import threading
import time
from dataclasses import dataclass
from urllib.parse import quote, urlencode


def stop(process):
    if process.poll() is not None:
        return
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    except PermissionError:
        # macOS can return EPERM when the group exits between poll and killpg.
        if process.poll() is None:
            raise
    try:
        process.wait(timeout=3)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.wait()


def watch_events(pipe, events):
    """kubectl pretty-prints a sequence of JSON objects, not JSON lines."""
    buffer = ""
    try:
        for line in pipe:
            buffer += line
            if len(buffer) > 16 * 1024 * 1024:
                raise RuntimeError("pod watch object exceeded 16 MiB")
            try:
                event = json.loads(buffer)
            except json.JSONDecodeError:
                continue
            events.put(event)
            buffer = ""
        if buffer.strip():
            raise RuntimeError("incomplete JSON from pod watch")
    except (OSError, ValueError, RuntimeError) as error:
        events.put(error)
    finally:
        events.put(None)


@dataclass
class Stream:
    pod: str
    container: str
    path: Path
    process: subprocess.Popen
    running: bool
    active: bool = True


class Collector:
    def __init__(self):
        self.output = Path(os.environ.get("AUTHZ_LOG_DIR", "dist/authz-logs"))
        self.namespace = os.environ.get("AUTHZ_NAMESPACE", "radius-system")
        self.streams = {}
        self.previous = set()
        self.events = queue.Queue()
        self.processes = []
        self.watch = None
        self.reader = None

    def spawn(self, args, **kwargs):
        process = subprocess.Popen(args, start_new_session=True, **kwargs)
        self.processes.append(process)
        return process

    def inventory(self):
        result = subprocess.run(
            ["kubectl", "get", "pods", "--namespace", self.namespace,
             "--request-timeout=20s", "-o", "json"],
            check=True, stdout=subprocess.PIPE, text=True, timeout=30,
        )
        return json.loads(result.stdout)

    def log_args(self, pod, container):
        return ["kubectl", "logs", pod, "--namespace", self.namespace,
                "--container", container, "--timestamps=true", "--tail=-1"]

    def snapshot(self, args, path):
        with path.open("ab") as output:
            subprocess.run(args + ["--request-timeout=20s"], stdout=output,
                           check=True, timeout=30)

    def follow(self, args, path):
        with path.open("ab") as output:
            return self.spawn(args + ["--follow"], stdout=output)

    def observe(self, pod, deleted=False):
        name = pod["metadata"]["name"]
        uid = pod["metadata"]["uid"]
        statuses = pod.get("status", {})
        statuses = (statuses.get("containerStatuses", []) +
                    statuses.get("initContainerStatuses", []) +
                    statuses.get("ephemeralContainerStatuses", []))
        for key, stream in self.streams.items():
            if key[0] == uid:
                stream.active = False
        for status in statuses:
            container = status["name"]
            restart = status["restartCount"]
            key = (uid, container, restart)
            args = self.log_args(name, container)
            path = self.output / f"{name}.{uid}.{container}.{restart}.log"
            if restart > 0 and key not in self.previous:
                self.snapshot(args + ["--previous"], path.with_suffix(".previous.log"))
                self.previous.add(key)
            state = status.get("state", {})
            if not ("running" in state or "terminated" in state):
                continue
            if key not in self.streams:
                # Verify initial access before starting tests. Following with no
                # since/tail cutoff then recovers logs written during startup.
                self.snapshot(args, path)
                process = self.follow(args, path)
                self.streams[key] = Stream(name, container, path, process,
                                           "running" in state)
            stream = self.streams[key]
            stream.active = not deleted
            stream.running = "running" in state

    def drain_events(self, stopping=False):
        while True:
            try:
                event = self.events.get_nowait()
            except queue.Empty:
                return
            if event is None:
                if not stopping:
                    raise RuntimeError("pod watch ended before the command")
            elif isinstance(event, Exception):
                raise event
            elif event["type"] in ("ADDED", "MODIFIED", "DELETED"):
                self.observe(event["object"], deleted=event["type"] == "DELETED")
            else:
                raise RuntimeError(f"unexpected pod watch event: {event}")

    def check_streams(self):
        for stream in self.streams.values():
            status = stream.process.poll()
            if status is None:
                continue
            if status != 0:
                raise RuntimeError(
                    f"log stream failed for {stream.pod}/{stream.container}: {status}")
            if stream.active and stream.running:
                # A clean EOF can occur without a restart. Re-read all available
                # logs on reconnect; duplicate lines are harmless for the gate.
                stream.process = self.follow(
                    self.log_args(stream.pod, stream.container), stream.path)

    def start(self):
        self.output.mkdir(parents=True, exist_ok=False)
        inventory = self.inventory()
        if not inventory["items"]:
            raise RuntimeError(f"no pods in {self.namespace}")
        version = inventory["metadata"]["resourceVersion"]
        watch_url = (f"/api/v1/namespaces/{quote(self.namespace, safe='')}/pods?"
                     + urlencode({"watch": "true", "resourceVersion": version,
                                  "timeoutSeconds": 5400}))
        self.watch = self.spawn(
            ["kubectl", "get", "--raw", watch_url, "--request-timeout=0"],
            stdout=subprocess.PIPE, text=True,
        )
        self.reader = threading.Thread(
            target=watch_events, args=(self.watch.stdout, self.events), daemon=True)
        self.reader.start()
        for pod in inventory["items"]:
            self.observe(pod)

    def finish(self):
        self.drain_events()
        if self.watch.poll() is not None:
            raise RuntimeError("pod watch exited before final collection")
        # Snapshot the remaining pods through the end of the command, before
        # intentionally stopping their live streams.
        inventory = self.inventory()
        for pod in inventory["items"]:
            self.observe(pod)
            uid = pod["metadata"]["uid"]
            for key, stream in self.streams.items():
                if key[0] == uid and stream.active:
                    self.snapshot(self.log_args(stream.pod, stream.container), stream.path)
        stop(self.watch)
        self.reader.join(timeout=5)
        if self.reader.is_alive():
            raise RuntimeError("pod watch reader did not stop")
        self.drain_events(stopping=True)
        self.check_streams()
        for stream in self.streams.values():
            if not stream.active:
                # Deleted/replaced pods have no final snapshot to fall back on.
                # Let their streams drain rather than truncating their last logs.
                if stream.process.wait(timeout=5) != 0:
                    raise RuntimeError(
                        f"retired log stream failed for {stream.pod}/{stream.container}")

    def close(self):
        while self.processes:
            stop(self.processes.pop())
        if self.reader:
            self.reader.join(timeout=5)
        if self.watch:
            self.watch.stdout.close()


def interrupted(signum, _frame):
    raise SystemExit(128 + signum)


def main():
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} command [args...]", file=sys.stderr)
        return 2
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    collector = Collector()
    try:
        collector.start()
        command = collector.spawn(sys.argv[1:])
        while command.poll() is None:
            collector.drain_events()
            collector.check_streams()
            time.sleep(.1)
        collector.finish()
        collector.close()
        (collector.output / "complete").touch()
        return command.returncode if command.returncode >= 0 else 128 - command.returncode
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.SubprocessError) as error:
        print(f"authz log collection failed: {error}", file=sys.stderr)
        return 2
    finally:
        collector.close()


if __name__ == "__main__":
    sys.exit(main())
