/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package deployment

// delegationEvaluatorFixture is exported with RADIUS_DELEGATION_FIXTURES for opt-in, unchanged-engine checks.
// Its mock backend proves expression evaluation and PUT grouping, not runtime recovery or inventory.
const delegationEvaluatorFixture = `import argparse
import copy
import http.server
import json
from pathlib import Path
import subprocess
import threading
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent
SCOPE = "/planes/radius/local/resourceGroups/fixture"
IMAGE = "ghcr.io/radius-project/deployment-engine@sha256:de78b4b34351a96fbc7f678f29fef39b46da8b977de1681e263454a9ffb68fb8"
REQUESTS, RESOURCES = [], {}
LOCK = threading.RLock()

def load_template(case):
    path = ROOT / (case + ".json")
    if path.exists():
        return json.loads(path.read_text())
    return json.loads((ROOT / "proof.json").read_text())["templates"][case]

def request(method, path, body=None):
    req = urllib.request.Request("http://127.0.0.1:18748" + path,
        data=json.dumps(body).encode() if body is not None else None,
        method=method, headers={"Content-Type": "application/json"})
    try:
        response = urllib.request.urlopen(req, timeout=60)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read()
        return response.status, json.loads(raw) if raw else {}

def poll(path):
    for _ in range(250):
        status, value = request("GET", path)
        if (value or {}).get("properties", {}).get("provisioningState") in ("Succeeded", "Failed", "Canceled"):
            return value
        time.sleep(0.2)
    raise AssertionError(("deployment timeout", status, value))

class Provider(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass
    def service(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"]))) if self.headers.get("Content-Length") else None
        path = self.path.split("?")[0]
        if "/planes/" in path:
            path = path[path.index("/planes/"):]
        with LOCK:
            REQUESTS.append({"method": self.command, "path": path, "body": copy.deepcopy(body)})
        if "/Microsoft.Resources/deployments/" in path:
            status, value = request(self.command, path + "?api-version=2020-10-01", body)
            if self.command == "PUT" and status in (200, 201, 202):
                value = poll(path + "?api-version=2020-10-01")
                status = 200 if value["properties"]["provisioningState"] == "Succeeded" else 500
        else:
            with LOCK:
                if self.command == "PUT":
                    value = body or {}
                    value.update({"id": path, "type": path.split("/providers/")[-1].rsplit("/", 1)[0], "name": path.rsplit("/", 1)[-1]})
                    properties = value.setdefault("properties", {})
                    properties.update({"endpoint": "computed-" + value["name"], "provisioningState": "Succeeded"})
                    for member in properties.get("__radiusDelegatingResources", []):
                        assert member is not None
                        name = member["id"].rsplit("/", 1)[-1]
                        RESOURCES[member["id"].lower()] = {"id": member["id"], "name": name, "type": member["type"],
                            "properties": {**member["properties"], "endpoint": "computed-" + name, "provisioningState": "Succeeded"}}
                    RESOURCES[path.lower()] = value
                    status = 200
                else:
                    value = RESOURCES.get(path.lower(), {"error": {"code": "NotFound", "message": "fixture resource absent"}})
                    status = 200 if path.lower() in RESOURCES else 404
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(value).encode())
    do_PUT = service
    do_GET = service
    do_POST = service

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--cases", default="indexed-targets,indexed-targets-offset,indexed-targets-child,indexed-targets-mapping,indexed-targets-conditional")
    args = parser.parse_args()
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 18749), Provider)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    name = "delegation-indexed-planner-31142213"
    log = (ROOT / "engine.log").open("w")
    process = subprocess.Popen(["docker", "run", "--rm", "--name", name, "--network", "host",
        "-e", "ASPNETCORE_ENVIRONMENT=Development", "-e", "ASPNETCORE_URLS=http://127.0.0.1:18748",
        "-e", "RADIUSBACKENDURL=http://127.0.0.1:18749", IMAGE], stdout=log, stderr=subprocess.STDOUT)
    summary = {"image": IMAGE, "cases": []}
    try:
        for _ in range(40):
            try:
                status, _ = request("GET", "/swagger/v1/swagger.json")
                if status == 200:
                    break
            except OSError:
                pass
            time.sleep(0.5)
        else:
            raise AssertionError("engine did not become ready")
        for case in args.cases.split(","):
            template = load_template(case)
            indexed = case.startswith("indexed-targets")
            matrix = [(True, 2, 4), (False, 2, 4), (True, 3, 2), (False, 0, 4)] if indexed else [(True, 2, 4), (False, 2, 4)]
            for enabled, target_count, source_count in matrix:
                with LOCK:
                    REQUESTS.clear()
                    RESOURCES.clear()
                label = f"{case}-{str(enabled).lower()}-{target_count}-{source_count}"
                path = SCOPE + "/providers/Microsoft.Resources/deployments/" + label + "?api-version=2020-10-01"
                parameters = {"enabled": {"value": enabled}, "count": {"value": 2}}
                if indexed:
                    parameters.update({"targetCount": {"value": target_count}, "sourceCount": {"value": source_count}})
                status, deployment = request("PUT", path, {"properties": {"mode": "Incremental", "template": template, "parameters": parameters,
                    "providerConfig": {"radius": {"type": "Radius", "value": {"scope": SCOPE}},
                        "deployments": {"type": "Microsoft.Resources", "value": {"scope": SCOPE}}}}})
                (ROOT / (label + "-result.json")).write_text(json.dumps({"label": label, "initialStatus": status, "deployment": deployment}, indent=2))
                assert status in (200, 201, 202), (status, deployment)
                deployment = poll(path)
                result = {"label": label, "deployment": deployment, "requests": copy.deepcopy(REQUESTS)}
                (ROOT / (label + "-result.json")).write_text(json.dumps(result, indent=2))
                assert deployment["properties"]["provisioningState"] == "Succeeded", deployment
                puts = [r for r in REQUESTS if r["method"] == "PUT" and "/targets/target" in r["path"]]
                active_targets = min(1, target_count) if case == "indexed-targets-conditional" else target_count
                assert len(puts) == (active_targets if indexed else 1), puts
                assert len({r["path"] for r in puts}) == len(puts)
                assert not [r for r in REQUESTS if r["method"] == "PUT" and "/routes/" in r["path"]]
                assert len([r for r in REQUESTS if r["method"] == "PUT" and r["path"].endswith("/targets/peer")]) == 1
                if indexed:
                    offset = 1 if case == "indexed-targets-offset" else 0
                    name_offset = 5 if offset else 0
                    assert len([r for r in REQUESTS if r["method"] == "PUT" and "/consumers/external" in r["path"]]) == 1
                    for index in range(active_targets):
                        put = next(r for r in puts if r["path"].endswith("/target" + str(index + name_offset)))
                        assert put["body"]["properties"]["image"] == "image" + str(index)
                        members = put["body"]["properties"]["__radiusDelegatingResources"]
                        mapping = [1, 0, 1, 0] if case == "indexed-targets-mapping" else [(i + offset) % target_count for i in range(source_count)]
                        expected = [i for i in range(source_count) if enabled and i != 3 and mapping[i] == index]
                        names = {f"route{i}" for i in expected}
                        if enabled and index == 1:
                            names.add("fixed-route")
                        assert {m["id"].rsplit("/", 1)[-1] for m in members} == names, members
                        assert len(members) == len(names)
                        for member in members:
                            assert member["properties"]["destination"] == put["path"], member
                            source_name = member["id"].rsplit("/", 1)[-1]
                            source_index = 0 if source_name == "fixed-route" else int(source_name[5:])
                            assert member["properties"]["computed"] == "computed-external"
                            if source_name != "fixed-route":
                                assert member["properties"]["port"] == 6379 + source_index
                                assert member["properties"]["literal"] == "reference(format('server[{0}]',copyIndex())).id"
                                assert member["properties"]["expressionLiteral"] == "reference(format('server[{0}]',copyIndex())).id-literal"
                    endpoint = "computed-route0" if case == "indexed-targets-conditional" else "computed-route1"
                    assert deployment["properties"]["outputs"]["endpoint"]["value"] == (endpoint if enabled else "absent")
                else:
                    assert len(puts[0]["body"]["properties"]["__radiusDelegatingResources"]) == (1 if enabled else 0)
                    assert deployment["properties"]["outputs"]["endpoint"]["value"] == ("computed-child-route" if enabled else "absent")
                    if case == "output-only-parameter":
                        assert deployment["properties"]["outputs"]["ownerEndpoint"]["value"] == "computed-target"
                summary["cases"].append({"label": label, "state": "Succeeded", "targetPuts": len(puts), "assertionsPassed": True})
                print(json.dumps(summary["cases"][-1]), flush=True)
        if "indexed-targets" in args.cases.split(","):
            for target_count in (0, 1):
                template = load_template("indexed-targets")
                template["outputs"] = {}
                label = f"invalid-unused-target-index-{target_count}"
                (ROOT / (label + ".json")).write_text(json.dumps(template, indent=2))
                with LOCK:
                    REQUESTS.clear()
                    RESOURCES.clear()
                path = SCOPE + "/providers/Microsoft.Resources/deployments/" + label + "?api-version=2020-10-01"
                status, deployment = request("PUT", path, {"properties": {"mode": "Incremental", "template": template,
                    "parameters": {"enabled": {"value": True}, "count": {"value": 2},
                        "targetCount": {"value": target_count}, "sourceCount": {"value": 4}},
                    "providerConfig": {"radius": {"type": "Radius", "value": {"scope": SCOPE}},
                        "deployments": {"type": "Microsoft.Resources", "value": {"scope": SCOPE}}}}})
                if status in (200, 201, 202):
                    deployment = poll(path)
                state = (deployment or {}).get("properties", {}).get("provisioningState")
                (ROOT / (label + "-result.json")).write_text(json.dumps({"initialStatus": status, "deployment": deployment, "requests": REQUESTS}, indent=2))
                assert status in (400, 500) or state in ("Failed", "Canceled"), (status, deployment)
                assert not [r for r in REQUESTS if r["method"] == "PUT"], REQUESTS
                summary["cases"].append({"label": label, "state": state or "Rejected", "initialStatus": status, "assertionsPassed": True})
                print(json.dumps(summary["cases"][-1]), flush=True)
    finally:
        (ROOT / "summary.json").write_text(json.dumps(summary, indent=2))
        templates, originals, metadata = {}, {}, {}
        for case in args.cases.split(","):
            try:
                templates[case] = load_template(case)
            except (FileNotFoundError, KeyError):
                continue
            original = ROOT / (case + "-original.json")
            if original.exists():
                originals[case] = json.loads(original.read_text())
            declared = ROOT / (case + "-metadata.json")
            if declared.exists():
                metadata[case] = json.loads(declared.read_text())
        results = {}
        for case in summary["cases"]:
            path = ROOT / (case["label"] + "-result.json")
            if path.exists():
                results[case["label"]] = json.loads(path.read_text())
        errors, version = [], None
        for line in (ROOT / "engine.log").read_text().splitlines():
            try:
                record = json.loads(line)
            except ValueError:
                continue
            version = version or record.get("version")
            if record.get("severity") == "ERROR":
                errors.append(record.get("message"))
        bundle = {"summary": summary, "engineVersion": version, "templates": templates, "originals": originals,
            "plannerMetadata": metadata, "results": results, "engineErrors": errors,
            "validationScope": "Mock backend: native evaluator, grouped PUTs and logical outputs; not runtime inventory/recovery.",
            "reproduce": "python3 evaluate.py --cases " + args.cases + " (evaluate.py and proof.json in the same folder; existing Docker image required)"}
        (ROOT / "proof.json").write_text(json.dumps(bundle, indent=2))
        subprocess.run(["docker", "stop", name], stdout=subprocess.DEVNULL, check=False)
        process.wait(timeout=30)
        server.shutdown()
        server.server_close()
        log.close()

if __name__ == "__main__":
    main()
`
