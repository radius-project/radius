"""Exercise developer tasks with native command fakes, without downloads or servers."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class TaskfileTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tools = tempfile.TemporaryDirectory(prefix="Task fake tools ")
        cls.addClassCleanup(cls.tools.cleanup)
        cls.task = shutil.which("task")
        if cls.task is None:
            raise RuntimeError("Task is required to test the developer commands")
        cls.host = json.loads(subprocess.check_output(
            ["go", "env", "-json", "GOHOSTOS", "GOHOSTARCH"], text=True,
        ))
        cls.suffix = ".exe" if cls.host["GOHOSTOS"] == "windows" else ""
        executable = Path(cls.tools.name) / f"tool{cls.suffix}"
        environment = dict(
            os.environ, GOOS=cls.host["GOHOSTOS"],
            GOARCH=cls.host["GOHOSTARCH"], CGO_ENABLED="0",
        )
        subprocess.run(
            ["go", "build", "-o", str(executable), str(ROOT / "build/task/testdata/tool.go")],
            env=environment, check=True, capture_output=True,
        )
        for name in ("go", "helm"):
            shutil.copy2(executable, Path(cls.tools.name) / f"{name}{cls.suffix}")

    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="Task source with spaces ")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        shutil.copy2(ROOT / "Taskfile.yml", self.root / "Taskfile.yml")
        (self.root / "deploy/Chart").mkdir(parents=True)
        for module in ("test/testrp", "test/magpiego"):
            (self.root / module).mkdir(parents=True)
        self.trace = self.root / "trace.jsonl"
        self.assets = self.root / "envtest assets"
        self.assets.mkdir()
        for name in ("kube-apiserver", "etcd"):
            (self.assets / f"{name}{self.suffix}").touch()
        # Both ambient and persisted cross-target defaults must yield local commands.
        target_os = "linux" if self.host["GOHOSTOS"] == "windows" else "windows"
        target_arch = "arm64" if self.host["GOHOSTARCH"] == "amd64" else "amd64"
        goenv = self.root / "cross-target.goenv"
        goenv.write_bytes(f"GOOS={target_os}\nGOARCH={target_arch}\n".encode())
        self.environment = dict(
            os.environ, PATH=self.tools.name + os.pathsep + os.defpath,
            GOOS=target_os, GOARCH=target_arch, GOENV=str(goenv),
            TASK_TEST_TRACE=str(self.trace), TASK_TEST_ASSETS=str(self.assets),
        )
        if self.host["GOHOSTOS"] == "windows":
            self.environment["LOCALAPPDATA"] = str(self.root / "local app data")
            self.environment.pop("HOME", None)
        else:
            self.environment["HOME"] = str(self.root / "profile")
            self.environment.pop("LOCALAPPDATA", None)
        self.environment.pop("TASK_TEST_FAIL", None)

    def run_task(self, *arguments, code=0, cwd=None):
        result = subprocess.run(
            [self.task, "--exit-code", *arguments], cwd=cwd or self.root,
            env=self.environment, text=True, capture_output=True,
        )
        self.assertEqual(code, result.returncode, result.stdout + result.stderr)
        self.output = result.stdout + result.stderr
        return [json.loads(line) for line in self.trace.read_text().splitlines()] if self.trace.exists() else []

    def test_default_build_compiles_all_three_modules(self):
        events = self.run_task("build")
        self.assertEqual(4, len(events))
        self.assertEqual(["list", "-f", "{{.Name}}", "./..."], events[0]["args"])
        for module, event in zip((".", "./test/testrp", "./test/magpiego"), events[1:]):
            self.assertEqual(
                ["-C", module, "build", "-o", str(self.root / "dist/dev"), "-v", "./..."],
                event["args"],
            )
        self.assertEqual(
            {f"{name}{self.suffix}" for name in ("rad", "testrp", "magpiego")},
            {p.name for p in (self.root / "dist/dev").iterdir()},
        )
        self.assertEqual(3, self.output.count("compiler stdout"))
        self.assertEqual(3, self.output.count("compiler stderr"))

    def test_selected_build_and_debug_flags_from_subdirectory(self):
        subdirectory = self.root / "directory with spaces"
        subdirectory.mkdir()
        events = self.run_task(
            "build", "PACKAGE=./cmd/controller", "--", "-gcflags=all=-N -l", cwd=subdirectory,
        )
        self.assertEqual(2, len(events))
        self.assertEqual(
            ["-C", ".", "build", "-o", str(self.root / "dist/dev"), "-v", "-gcflags=all=-N -l", "./cmd/controller"],
            events[1]["args"],
        )
        self.assertEqual(self.root.resolve(), Path(events[0]["cwd"]).resolve())
        self.assertTrue((self.root / "dist/dev" / f"controller{self.suffix}").is_file())

    def test_default_build_forwards_flags_to_every_module(self):
        events = self.run_task("build", "--", "-gcflags=all=-N -l", "-trimpath")
        self.assertEqual(4, len(events))
        for event in events:
            self.assertEqual(["-gcflags=all=-N -l", "-trimpath", "./..."], event["args"][-3:])

    def test_selected_library_compiles_without_requesting_an_executable(self):
        events = self.run_task("build", "PACKAGE=./pkg/version")
        self.assertEqual(2, len(events))
        self.assertEqual(["-C", ".", "build", "-v", "./pkg/version"], events[1]["args"])
        self.assertEqual([], list((self.root / "dist/dev").iterdir()))

    def test_install_builds_rad_and_uses_the_official_user_profile_destination(self):
        destination = (
            self.root / "local app data" / "radius" / f"rad{self.suffix}"
            if self.host["GOHOSTOS"] == "windows"
            else self.root / "profile" / ".local/bin" / f"rad{self.suffix}"
        )
        events = self.run_task("install")
        self.assertEqual(2, len(events))
        self.assertEqual(["list", "-f", "{{.Name}}", "./cmd/rad"], events[0]["args"])
        self.assertEqual(["-C", ".", "build", "-o", str(self.root / "dist/dev"), "-v", "./cmd/rad"], events[1]["args"])
        self.assertEqual(b"test binary", destination.read_bytes())

        destination.write_bytes(b"old installation")
        self.run_task("install")
        self.assertEqual(b"test binary", destination.read_bytes())

    def test_install_dir_override_is_supported(self):
        destination = self.root / "custom install" / f"rad{self.suffix}"
        self.run_task("install", f"INSTALL_DIR={destination.parent}")
        self.assertEqual(b"test binary", destination.read_bytes())

    def test_go_tests_provision_assets_without_helm_or_python(self):
        for arguments, packages_and_flags in (
            ((), ["./pkg/..."]),
            (("PACKAGE=./test/validation/...", "--", "-run", "Test name with spaces", "-count=1"),
             ["./test/validation/...", "-run", "Test name with spaces", "-count=1"]),
        ):
            with self.subTest(arguments=arguments):
                self.trace.unlink(missing_ok=True)
                events = self.run_task("test", *arguments)
                self.assertEqual(["go", "go"], [e["tool"] for e in events])
                self.assertEqual(
                    ["tool", "setup-envtest", "use", "-p", "path", "1.30.*", "--arch", "amd64"],
                    events[0]["args"],
                )
                self.assertEqual(["tool", "gotestsum", "--", *packages_and_flags], events[1]["args"])
                self.assertEqual(str(self.assets), events[1]["assets"])
                self.assertEqual("1", events[1]["cgo"])
                for event in events:
                    self.assertEqual(self.host["GOHOSTOS"], event["goos"])
                    self.assertEqual(self.host["GOHOSTARCH"], event["goarch"])

    def test_helm_tests_are_explicit_and_do_not_install_plugins(self):
        events = self.run_task("test:helm")
        self.assertEqual(1, len(events))
        self.assertEqual(("helm", ["unittest", "."]), (events[0]["tool"], events[0]["args"]))
        self.assertEqual((self.root / "deploy/Chart").resolve(), Path(events[0]["cwd"]).resolve())

    def test_command_failures_are_not_skipped(self):
        for failure, task, code, calls in (
            ("build", "build", 42, 2),
            ("module", "build", 42, 3),
            ("build", "install", 42, 2),
            ("test", "test", 43, 2),
            ("helm", "test:helm", 44, 1),
            ("setup-envtest", "test", 1, 1),
        ):
            with self.subTest(failure=failure):
                self.trace.unlink(missing_ok=True)
                self.environment["TASK_TEST_FAIL"] = failure
                events = self.run_task(task, code=code)
                self.assertEqual(calls, len(events))

    def test_help_does_not_invoke_external_tools(self):
        self.assertEqual([], self.run_task())
        for name in ("build:", "install:", "test:", "test:helm:"):
            self.assertIn(name, self.output)


if __name__ == "__main__":
    unittest.main()
