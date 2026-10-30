#!/usr/bin/env python3
"""Repeatable, assertion-backed demo through the real Radius CLI and APIs."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.request
import zipfile


ROOT = Path(__file__).resolve().parents[3]
RUN = Path("/home/brhamilt/delegated-provisioning-deebec8c-run")
CLUSTER = "delegated-deebec8c"
NETWORK = "delegated-net-deebec8c"
SCOPE = "/planes/radius/local/resourceGroups/delegation-demo"
ENVIRONMENT = SCOPE + "/providers/Radius.Core/environments/demo"
APPLICATION = SCOPE + "/providers/Radius.Core/applications/demo"
TARGET = SCOPE + "/providers/Radius.Compute/containers/cache"
PACK = SCOPE + "/providers/Radius.Core/recipePacks/delegated-kubernetes"
SETTINGS = SCOPE + "/providers/Radius.Core/terraformSettings/demo-offline"
API = "2025-08-01-preview"
PREFIX = "http://127.0.0.1:18586/apis/api.ucp.dev/v1alpha3"
RAD = ROOT / "dist/linux_amd64/release/rad"
os.environ["KUBECONFIG"] = str(RUN / "kubeconfig")
os.environ["HOME"] = str(RUN / "home")
os.environ["PATH"] = "/home/brhamilt/.local/bin:/usr/local/bin:/usr/bin:/bin"
EVIDENCE = RUN / "evidence"


def command(*args, json_output=False):
    result = subprocess.run(
        [str(arg) for arg in args], check=False, text=True, capture_output=True
    )
    if result.stderr:
        print(result.stderr, end="", flush=True)
    if result.returncode != 0:
        raise RuntimeError(f"command failed ({result.returncode}): {args}\n{result.stdout}\n{result.stderr}")
    if json_output:
        return json.loads(result.stdout)
    return result.stdout.strip()


def api(method, resource_id, body=None, version=API, missing_ok=False):
    request = urllib.request.Request(
        PREFIX + resource_id + "?api-version=" + version,
        data=json.dumps(body).encode() if body is not None else None,
        method=method,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            data = response.read()
            return json.loads(data) if data else None
    except urllib.error.HTTPError as error:
        if missing_ok and error.code == 404:
            return None
        raise RuntimeError(
            f"{method} {resource_id}: HTTP {error.code}: {error.read().decode()}"
        ) from error


def wait(resource_id, deleting=False, version=API):
    deadline = time.monotonic() + 300
    while time.monotonic() < deadline:
        resource = api("GET", resource_id, version=version, missing_ok=deleting)
        if resource is None and deleting:
            return
        state = resource.get("properties", {}).get("provisioningState")
        if state in ("Failed", "Canceled"):
            raise RuntimeError(json.dumps(resource, indent=2))
        if not deleting and state in (None, "Succeeded"):
            return resource
        time.sleep(2)
    raise TimeoutError(f"{resource_id} did not settle")


def route_id(name):
    return SCOPE + "/providers/Radius.Compute/routes/" + name


def create(resource_type, name, properties, expect_error=None):
    path = RUN / (name + "-request.json")
    path.write_text(json.dumps({"properties": properties}, indent=2))
    print(f"$ rad resource create {resource_type} {name} -f {path.name}", flush=True)
    args = [str(RAD), "resource", "create", resource_type, name, "-f", str(path), "-o", "json"]
    if expect_error:
        result = subprocess.run(args, text=True, capture_output=True, check=False)
        assert result.returncode != 0, "invalid request unexpectedly succeeded"
        error_output = result.stdout + result.stderr
        assert expect_error in error_output, error_output
        failed = api("GET", SCOPE + "/providers/" + resource_type + "/" + name)
        assert failed["properties"]["provisioningState"] == "Failed"
        (EVIDENCE / (name + "-expected-failure.txt")).write_text(error_output)
        print(f"PASS expected rejection: {expect_error}", flush=True)
        return failed
    result = command(*args)
    (EVIDENCE / (name + "-cli-last.json")).write_text(result + "\n")
    return wait(SCOPE + "/providers/" + resource_type + "/" + name)


def owner(image="nginx:1.27-alpine"):
    return {
        "environment": ENVIRONMENT,
        "application": APPLICATION,
        "containers": {
            "web": {
                "image": image,
                "ports": {
                    "http": {"containerPort": 80},
                    "alternate": {"containerPort": 8080},
                    "metrics": {"containerPort": 9090},
                },
            }
        },
    }


def route(port):
    return {
        "environment": ENVIRONMENT,
        "application": APPLICATION,
        "kind": "TCP",
        "rules": [{
            "matches": [],
            "destinationContainer": {
                "resourceId": TARGET, "containerName": "web", "containerPort": port
            },
        }],
    }


def native(kind, name, missing=False):
    args = ["kubectl", "-n", "delegated-demo", "get", kind, name, "-o", "json"]
    if missing:
        args.append("--ignore-not-found")
    output = command(*args)
    return json.loads(output) if output else None


def snapshot(label, expected_ports):
    target = api("GET", TARGET)
    deployment = native("deployment", "delegated-cache")
    service = native("service", "delegated-cache", missing=True)
    ports = sorted(port["port"] for port in service["spec"]["ports"]) if service else []
    assert ports == sorted(expected_ports), (label, ports, expected_ports)
    reconciliation = target["properties"]["status"]["reconciliation"]
    assert reconciliation["desiredRevision"] == reconciliation["appliedRevision"]
    routes = {}
    for name in ("web", "metrics"):
        resource = api("GET", route_id(name), missing_ok=True)
        if resource:
            assert not resource["properties"].get("status", {}).get("outputResources")
            routes[name] = resource
    record = {
        "target": target, "routes": routes,
        "deployment": deployment, "service": service,
    }
    (EVIDENCE / (label + ".json")).write_text(json.dumps(record, indent=2) + "\n")
    print(
        f"PASS {label}: native ports={ports}; deployment UID={deployment['metadata']['uid']}; "
        f"target inventory={len(target['properties']['status'].get('outputResources', []))}",
        flush=True,
    )
    return record


def delete_route(name):
    print(f"$ rad resource delete Radius.Compute/routes {name} --yes", flush=True)
    command(RAD, "resource", "delete", "Radius.Compute/routes", name, "--yes")
    wait(route_id(name), deleting=True)


def probe(port):
    command(
        "kubectl", "-n", "delegated-demo", "run", "delegated-probe",
        "--image=nginx:1.28-alpine", "--image-pull-policy=IfNotPresent",
        "--restart=Never",
        "--command", "--", "wget", "-qO-", f"http://delegated-cache:{port}/",
    )
    try:
        command(
            "kubectl", "-n", "delegated-demo", "wait",
            "--for=jsonpath={.status.phase}=Succeeded", "pod/delegated-probe",
            "--timeout=60s",
        )
        output = command("kubectl", "-n", "delegated-demo", "logs", "delegated-probe")
    finally:
        command("kubectl", "-n", "delegated-demo", "delete", "pod", "delegated-probe", "--wait=true")
    assert "delegated provisioning demo" in output, output
    print(f"PASS live HTTP-over-TCP probe through native Service port {port}", flush=True)


def reset():
    for name in ("invalid", "repair", "web", "metrics"):
        if api("GET", route_id(name), missing_ok=True):
            delete_route(name)
    if api("GET", TARGET, missing_ok=True):
        command(RAD, "resource", "delete", "Radius.Compute/containers", "cache", "--yes")
        wait(TARGET, deleting=True)
    assert native("deployment", "delegated-cache", missing=True) is None
    assert native("service", "delegated-cache", missing=True) is None
    print("PASS reset: target native Deployment and Service removed", flush=True)


def prepare_recipe():
    modules = RUN / "modules"
    modules.mkdir(exist_ok=True)
    archive = modules / "recipe.zip"
    with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as output:
        def add_file(path, name):
            entry = zipfile.ZipInfo(name, (2026, 1, 1, 0, 0, 0))
            entry.compress_type = zipfile.ZIP_DEFLATED
            entry.external_attr = (path.stat().st_mode & 0xFFFF) << 16
            output.writestr(entry, path.read_bytes())

        add_file(Path(__file__).parent / "recipe/main.tf", "main.tf")
        mirror = RUN / "tools/provider-mirror"
        for path in sorted(mirror.rglob("*")):
            if path.is_file():
                add_file(path, "providers/" + str(path.relative_to(mirror)))
        output.writestr(
            zipfile.ZipInfo("terraform.rc", (2026, 1, 1, 0, 0, 0)),
            'provider_installation {\n'
            '  filesystem_mirror {\n'
            '    path = ".terraform/modules/default/providers"\n'
            '    include = ["registry.terraform.io/hashicorp/kubernetes"]\n'
            '  }\n'
            '}\n',
        )
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    pinned = modules / (digest + ".zip")
    if not pinned.exists():
        pinned.write_bytes(archive.read_bytes())
    container_name = "delegated-recipes-deebec8c"
    existing = command("docker", "ps", "-a", "--filter", "name=^/" + container_name + "$", "--format", "{{.Names}}")
    if not existing:
        command(
            "docker", "run", "-d", "--name", container_name, "--network", NETWORK,
            "--mount", f"type=bind,src={modules},dst=/usr/share/nginx/html,readonly",
            "nginx:1.27-alpine",
        )
    inspect = command("docker", "inspect", container_name, json_output=True)[0]
    assert inspect["State"]["Running"], "owned recipe server is not running"
    assert inspect["Mounts"][0]["Source"] == str(modules)
    address = inspect["NetworkSettings"]["Networks"][NETWORK]["IPAddress"]
    source = f"http://{address}/{digest}.zip"
    (EVIDENCE / "recipe.json").write_text(json.dumps({"source": source, "sha256": digest}, indent=2) + "\n")
    return source


def initialize():
    for _ in range(90):
        try:
            api(
                "GET",
                "/planes/radius/local/providers/System.Resources/resourceproviders/Radius.Compute/resourcetypes/routes",
                version="2023-10-01-preview",
            )
            break
        except RuntimeError:
            time.sleep(2)
    else:
        raise RuntimeError("Radius.Compute/routes was not registered")
    if not command("kubectl", "get", "namespace", "delegated-demo", "--ignore-not-found"):
        command("kubectl", "create", "namespace", "delegated-demo")
    api("PUT", SCOPE, {"location": "global", "properties": {}}, version="2023-10-01-preview")
    source = prepare_recipe()
    digest = source.rsplit("/", 1)[1].removesuffix(".zip")
    pack_id = PACK + "-" + digest[:12]
    current_environment = api("GET", ENVIRONMENT, missing_ok=True)
    if current_environment and current_environment["properties"].get("recipePacks") != [pack_id]:
        reset()
    api("PUT", pack_id, {
        "location": "global",
        "properties": {"recipes": {
            "Radius.Compute/containers": {
                "kind": "terraform", "source": source,
                "acceptsDelegationFrom": ["Radius.Compute/routes"],
            },
            "Radius.Compute/routes": {
                "kind": "delegated",
                "target": {
                    "resourceType": "Radius.Compute/containers",
                    "idsFrom": "$.rules[*].destinationContainer.resourceId",
                },
            },
        }},
    })
    api("PUT", SETTINGS, {
        "location": "global",
        "properties": {
            "env": {"TF_CLI_CONFIG_FILE": ".terraform/modules/default/terraform.rc"}
        },
    })
    api("PUT", ENVIRONMENT, {
        "location": "global",
        "properties": {
            "recipePacks": [pack_id],
            "terraformSettings": SETTINGS,
            "providers": {"kubernetes": {"namespace": "delegated-demo"}},
        },
    })
    api("PUT", APPLICATION, {"location": "global", "properties": {"environment": ENVIRONMENT}})
    wait(APPLICATION)
    command(
        RAD, "workspace", "create", "kubernetes", "delegated-demo",
        "--context", "kind-" + CLUSTER, "--group", "delegation-demo",
        "--environment", "demo", "--force",
    )


def demo():
    reset()
    create("Radius.Compute/containers", "cache", owner())
    initial = snapshot("01-owner", [])
    deployment_uid = initial["deployment"]["metadata"]["uid"]
    create("Radius.Compute/routes", "web", route(80))
    create("Radius.Compute/routes", "metrics", route(9090))
    first = snapshot("02-two-routes", [80, 9090])
    assert first["routes"]["web"]["properties"]["listener"]["port"] == 80
    assert first["routes"]["metrics"]["properties"]["listener"]["port"] == 9090
    probe(80)
    rejected = subprocess.run(
        [str(RAD), "resource", "delete", "Radius.Compute/containers", "cache", "--yes"],
        text=True, capture_output=True, check=False,
    )
    error_output = rejected.stdout + rejected.stderr
    assert rejected.returncode != 0 and "target has delegation bindings" in error_output, error_output
    (EVIDENCE / "target-delete-expected-failure.txt").write_text(error_output)
    assert native("deployment", "delegated-cache")["metadata"]["uid"] == deployment_uid
    print("PASS target deletion rejected while delegates remain", flush=True)
    invalid = route(80)
    invalid["rules"].append({
        "matches": [],
        "destinationContainer": {
            "resourceId": TARGET + "-other", "containerName": "web", "containerPort": 8080
        },
    })
    create("Radius.Compute/routes", "invalid", invalid, expect_error="exactly one target")
    delete_route("invalid")
    create("Radius.Compute/routes", "repair", route(80), expect_error="unique TCP ports")
    create("Radius.Compute/routes", "repair", route(8080))
    delete_route("repair")
    snapshot("02b-rejected-and-corrected", [80, 9090])
    create("Radius.Compute/containers", "cache", owner("nginx:1.28-alpine"))
    updated = snapshot("03-owner-only-update", [80, 9090])
    assert updated["deployment"]["spec"]["template"]["spec"]["containers"][0]["image"] == "nginx:1.28-alpine"
    create("Radius.Compute/routes", "web", route(8080))
    updated = snapshot("04-route-only-update", [8080, 9090])
    assert updated["routes"]["web"]["properties"]["listener"]["port"] == 8080
    assert updated["deployment"]["metadata"]["uid"] == deployment_uid
    probe(8080)
    delete_route("web")
    snapshot("05-delete-one-route", [9090])
    delete_route("metrics")
    final = snapshot("06-delete-last-route", [])
    assert final["deployment"]["metadata"]["uid"] == deployment_uid
    command("kubectl", "-n", "delegated-demo", "rollout", "status", "deployment/delegated-cache", "--timeout=120s")
    print("PASS demo: all native changes were triggered by independent Radius CRUD", flush=True)


def provenance():
    paths = sorted(set(
        command("git", "-C", ROOT, "diff", "--name-only").splitlines()
        + command("git", "-C", ROOT, "ls-files", "--others", "--exclude-standard").splitlines()
    ))
    files = {
        path: hashlib.sha256((ROOT / path).read_bytes()).hexdigest()
        for path in paths if (ROOT / path).is_file() and "__pycache__" not in Path(path).parts
    }
    pods = command("kubectl", "-n", "radius-system", "get", "pods", "-o", "json", json_output=True)
    images = []
    for pod in pods["items"]:
        for container in pod.get("status", {}).get("containerStatuses", []):
            images.append({
                "pod": pod["metadata"]["name"],
                "container": container["name"],
                "image": container["image"],
                "imageID": container["imageID"],
                "ready": container["ready"],
            })
    record = {
        "baseCommit": command("git", "-C", ROOT, "rev-parse", "HEAD"),
        "sourceFilesSHA256": files,
        "cliSHA256": hashlib.sha256(RAD.read_bytes()).hexdigest(),
        "runtimeImages": images,
        "cluster": CLUSTER,
        "network": NETWORK,
        "scope": SCOPE,
        "target": "local Kubernetes; not ACA",
    }
    (EVIDENCE / "provenance.json").write_text(json.dumps(record, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--reset", action="store_true")
    args = parser.parse_args()
    EVIDENCE.mkdir(exist_ok=True)
    assert command("kubectl", "config", "current-context") == "kind-" + CLUSTER
    log = (RUN / "proxy.log").open("w")
    proxy = subprocess.Popen(
        ["kubectl", "proxy", "--port=18586", "--address=127.0.0.1"],
        stdout=log, stderr=subprocess.STDOUT,
    )
    try:
        for _ in range(50):
            if proxy.poll() is not None:
                raise RuntimeError("kubectl proxy exited; port 18586 must be free")
            try:
                urllib.request.urlopen("http://127.0.0.1:18586/version", timeout=1).close()
                break
            except (OSError, urllib.error.URLError):
                time.sleep(0.2)
        else:
            raise RuntimeError("kubectl proxy did not become ready")
        initialize()
        if args.reset:
            reset()
        else:
            demo()
        provenance()
    finally:
        proxy.terminate()
        proxy.wait(timeout=10)
        log.close()


if __name__ == "__main__":
    main()
