import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("get_release_version.py")


class GetReleaseVersionTests(unittest.TestCase):
    def run_script(self, git_ref, versions=None):
        with tempfile.TemporaryDirectory() as temp_dir:
            temp_path = Path(temp_dir)
            github_env = temp_path / "github-env"
            versions_file = temp_path / "versions.yaml"
            versions_file.write_text(
                versions or (
                    "supported:\n"
                    "  - channel: '0.60'\n"
                    "    version: 'v0.60.0-rc1'\n"
                    "  - channel: '0.59'\n"
                    "    version: 'v0.59.0'\n"
                    "  - channel: '0.58'\n"
                    "    version: 'v0.58.0'\n"
                ),
                encoding="utf-8",
            )

            environment = os.environ.copy()
            environment.update({
                "GITHUB_ENV": str(github_env),
                "GITHUB_REF": git_ref,
                "VERSIONS_FILE": str(versions_file),
            })
            subprocess.run(
                [sys.executable, str(SCRIPT_PATH)],
                check=True,
                env=environment,
                capture_output=True,
                text=True,
            )

            return dict(
                line.split("=", 1)
                for line in github_env.read_text(encoding="utf-8").splitlines()
            )

    def test_latest_stable_release_updates_latest(self):
        values = self.run_script("refs/tags/v0.59.0")

        self.assertEqual("0.59", values["REL_CHANNEL"])
        self.assertEqual("true", values["UPDATE_RELEASE"])
        self.assertEqual("true", values["UPDATE_CHANNEL"])
        self.assertEqual("true", values["UPDATE_LATEST"])

    def test_stale_patch_in_latest_channel_does_not_update_latest(self):
        values = self.run_script(
            "refs/tags/v0.59.1",
            "supported:\n"
            "  - channel: '0.60'\n"
            "    version: 'v0.60.0-rc1'\n"
            "  - channel: '0.59'\n"
            "    version: 'v0.59.2'\n",
        )

        self.assertEqual("0.59", values["REL_CHANNEL"])
        self.assertEqual("0.59.2", values["LATEST_STABLE_VERSION"])
        self.assertEqual("false", values["UPDATE_CHANNEL"])
        self.assertEqual("false", values["UPDATE_LATEST"])

    def test_older_channel_full_release_does_not_update_latest(self):
        values = self.run_script("refs/tags/v0.58.0")

        self.assertEqual("0.58", values["REL_CHANNEL"])
        self.assertEqual("true", values["UPDATE_RELEASE"])
        self.assertEqual("true", values["UPDATE_CHANNEL"])
        self.assertEqual("false", values["UPDATE_LATEST"])

    def test_newly_finalized_channel_updates_latest(self):
        values = self.run_script(
            "refs/tags/v0.60.0",
            "supported:\n"
            "  - channel: '0.60'\n"
            "    version: 'v0.60.0'\n"
            "  - channel: '0.59'\n"
            "    version: 'v0.59.0'\n",
        )

        self.assertEqual("0.60", values["LATEST_STABLE_CHANNEL"])
        self.assertEqual("true", values["UPDATE_CHANNEL"])
        self.assertEqual("true", values["UPDATE_LATEST"])

    def test_prerelease_does_not_update_latest(self):
        values = self.run_script("refs/tags/v0.60.0-rc1")

        self.assertEqual("0.60.0-rc1", values["REL_CHANNEL"])
        self.assertNotIn("UPDATE_RELEASE", values)
        self.assertEqual("false", values["UPDATE_CHANNEL"])
        self.assertEqual("false", values["UPDATE_LATEST"])

    def test_main_build_uses_edge_channel(self):
        values = self.run_script("refs/heads/main")

        self.assertEqual("edge", values["REL_VERSION"])
        self.assertEqual("edge", values["REL_CHANNEL"])
        self.assertEqual("0.59", values["LATEST_STABLE_CHANNEL"])
        self.assertEqual("0.59.0", values["LATEST_STABLE_VERSION"])
        self.assertNotIn("UPDATE_RELEASE", values)
        self.assertEqual("false", values["UPDATE_CHANNEL"])
        self.assertEqual("false", values["UPDATE_LATEST"])


if __name__ == "__main__":
    unittest.main()