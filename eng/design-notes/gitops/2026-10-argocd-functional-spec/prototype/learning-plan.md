# Argo CD learning plan, then a guided Radius plugin prototype

You run every command; I explain and answer questions. Each step has a
**Goal**, **Do**, **Check** and **Notice** (the thing to learn). Steps marked
🛑 are checkpoints: tell me what you saw before moving on.

Setup log for anything already installed:
[argocd-prototype.md](argocd-prototype.md).

## Where things stand

- Cluster `kind-kind-argo` (arm64), with Radius 0.61.1 and Argo CD v3.5.5
  installed by Helm (release `argocd`, values in `~/argocd-values.yaml`,
  which now holds the `DeploymentTemplate` Lua health check).
- **Resume here (2026-10-23):** A0–A6 done; A7 skipped (concept only, see
  Learnings). Start at **A8 step 1**: rewrite `~/argocd-values.yaml` with the
  `txt2cm` plugin (keep the health check), `helm upgrade`, and check the
  repo-server pod has two containers, `repo-server` and `txt2cm`. Then Part B.
- UI: <http://localhost:8090> (not 8080, which is something else on your
  Mac). User `admin`; password:
  `kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d`.
- CLI: run this in each new terminal:

  ```bash
  kubectl config use-context kind-kind-argo
  export ARGOCD_OPTS="--port-forward --port-forward-namespace argocd --kube-context kind-kind-argo"
  argocd login --username admin --password <password> --plaintext
  ```

- If the UI stops loading, restart the port-forward loop:

  ```bash
  while true; do kubectl -n argocd port-forward svc/argocd-server 8090:80; sleep 1; done
  ```

---

## Part A — Argo CD hands-on (about 2 hours, no Radius)

### A0. Install Argo CD yourself and open the UI (15 min)

**Goal:** do the install end to end. (I installed it yesterday; step 1 removes
that copy so you start clean.)

**Do:**

```bash
# 1. Remove the existing install (skip on a fresh cluster)
helm uninstall argocd -n argocd --kube-context kind-kind-argo
kubectl --context kind-kind-argo delete namespace argocd

# 2. Get the chart
helm repo add argo https://argoproj.github.io/argo-helm
helm repo update argo

# 3. Values: serve plain HTTP so a local port-forward works without TLS
cat > ~/argocd-values.yaml <<'EOF'
configs:
  params:
    server.insecure: true
EOF

# 4. Install (Argo CD v3.5.5)
helm install argocd argo/argo-cd --version 10.10.5 \
  -n argocd --create-namespace -f ~/argocd-values.yaml \
  --kube-context kind-kind-argo --wait

# 5. Check: every pod Running (redis-secret-init shows Completed)
kubectl --context kind-kind-argo -n argocd get pods

# 6. Admin password
kubectl --context kind-kind-argo -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath="{.data.password}" | base64 -d; echo

# 7. Port-forward (leave this terminal open)
kubectl --context kind-kind-argo -n argocd port-forward svc/argocd-server 8090:80
```

Open <http://localhost:8090> and log in as `admin` with the password from
step 6.

If the port-forward exits with "lost connection to pod", run it again. Or
keep it in a loop:
`while true; do kubectl --context kind-kind-argo -n argocd port-forward svc/argocd-server 8090:80; sleep 1; done`

**Notice:** `argocd-server` is a ClusterIP Service, so nothing is reachable
from your Mac until you port-forward (or add an Ingress or LoadBalancer).

### A1. Meet the components (10 min)

**Goal:** know which pod does what, because the plugin lives in one of them.

**Do:**

```bash
kubectl -n argocd get pods
kubectl -n argocd get deploy argocd-repo-server -o jsonpath='{.spec.template.spec.containers[*].name}'; echo
kubectl -n argocd get cm argocd-cm -o yaml | head -30
```

**Notice:**

| Pod                             | Job                                                                                                         |
|---------------------------------|-------------------------------------------------------------------------------------------------------------|
| `argocd-repo-server`            | Clones Git and **renders** manifests (plain YAML, Helm, Kustomize, or a plugin). Never touches the cluster. |
| `argocd-application-controller` | Compares rendered manifests with the cluster, **applies** them, and computes **health**.                    |
| `argocd-server`                 | API, UI and CLI backend.                                                                                    |
| `argocd-cm` ConfigMap           | Global settings, including custom Lua health checks.                                                        |

Rendering and applying happen in different pods. Our plugin only renders.

### A2. Your first Application, through the UI (15 min)

**Goal:** see the Application object and the sync loop.

**Do:** in the UI, **+ New App**:

- Name `guestbook`, project `default`, sync policy **Manual**.
- Repo `https://github.com/argoproj/argocd-example-apps.git`, revision `HEAD`,
  path `guestbook`.
- Cluster `https://kubernetes.default.svc`, namespace `guestbook`; tick
  **Auto-create namespace**.
- Create, then **Sync**.

**Check:** the app turns **Synced** and **Healthy**; the tree shows a
Deployment, ReplicaSet, Pod and Service.

**Notice:** before you click Sync it is **OutOfSync** and **Missing**. Sync
status (does the cluster match Git?) and health (is it working?) are two
separate axes.

### A3. The same Application, as YAML (10 min)

**Goal:** an Application is just a Kubernetes resource; this is how we'll
create the Radius ones.

**Do:**

```bash
kubectl -n argocd get application guestbook -o yaml
argocd app get guestbook
argocd app manifests guestbook | head -40     # what repo-server rendered
```

**Notice:** `spec.source` (where), `spec.destination` (to which
cluster and namespace), `spec.syncPolicy`, and `status.sync` /
`status.health` / `status.resources`.

### A4. Drift, diff and self-heal (15 min)

**Goal:** see what Argo CD does when the cluster drifts from Git.

**Do:**

```bash
kubectl -n guestbook scale deploy guestbook-ui --replicas=3
argocd app get guestbook        # OutOfSync
argocd app diff guestbook
argocd app set guestbook --sync-policy automated --self-heal
kubectl -n guestbook get deploy guestbook-ui -w   # back to 1
```

**Notice:** self-heal reverts manual changes. With Radius, Argo CD will
self-heal the `DeploymentTemplate`, but **not** the Deployments Radius creates,
because Argo CD doesn't own those.

### A5. Your own repo, commits and prune (25 min)

**Goal:** drive changes through Git, and see deletion (prune).

**Do:**

1. Create a public repo, e.g. `gh repo create nithyatsu/argocd-playground --public --clone`.
2. Add `plain/nginx.yaml` with a Deployment (nginx, 1 replica) and a
   ConfigMap named `hello`. Commit and push.
3. Create the Application declaratively:

   ```yaml
   # playground-app.yaml
   apiVersion: argoproj.io/v1alpha1
   kind: Application
   metadata:
     name: playground
     namespace: argocd
   spec:
     project: default
     source:
       repoURL: https://github.com/nithyatsu/argocd-playground.git
       targetRevision: main
       path: plain
     destination:
       server: https://kubernetes.default.svc
       namespace: playground
     syncPolicy:
       automated: {}
       syncOptions: [CreateNamespace=true]
   ```

   `kubectl apply -f playground-app.yaml`
4. Change the replicas to 2, push, then run `argocd app get playground --hard-refresh`.
5. Delete the ConfigMap from Git and push. It stays in the cluster, shown as
   requiring pruning. Then `argocd app set playground --auto-prune` and watch
   it go.

**Notice:**

- Argo CD polls Git about every 3 minutes; `--refresh` and `--hard-refresh` force it.
- Pruning is opt-in.
- Every resource Argo CD applies carries a tracking marker:
  `kubectl -n playground get cm -o yaml | grep -i argocd`.
  That marker decides what shows up in the tree.

### A6. Health of a resource Argo CD doesn't know (20 min) 🛑

**Goal:** write a Lua health check. This is exactly what we'll do for
`DeploymentTemplate`.

**Do:**

1. `kubectl get crd | grep radapp`. Radius's `DeploymentTemplate` CRD is
   already installed.
2. Add `plain/fake-dt.yaml` to your playground repo: a `DeploymentTemplate`
   with an empty template is fine. Push, sync, and look at its health in the
   UI. It shows no health, because Argo CD doesn't know this kind.
3. Add a health check to Argo CD. Edit
   `~/argocd-values.yaml` (copy: `argocd-values.yaml` in this folder):

   ```yaml
   configs:
     params:
       server.insecure: true
     cm:
       resource.customizations.health.radapp.io_DeploymentTemplate: |
         hs = {}
         if obj.status ~= nil and obj.status.phrase == "Ready" then
           hs.status = "Healthy"; hs.message = "Radius says Ready"
         elseif obj.status ~= nil and obj.status.phrase == "Failed" then
           hs.status = "Degraded"; hs.message = "Radius says Failed"
         else
           hs.status = "Progressing"; hs.message = "Waiting for Radius"
         end
         return hs
   ```

   Apply it with `helm upgrade argocd argo/argo-cd --version 10.10.5 -n argocd -f <values>`,
   then hard-refresh the app.

**Check:** the `DeploymentTemplate` node now shows a health state, and it
rolls up to the Application.

**Notice:** the key name `<group>_<Kind>`, and that Lua only sees `obj`, the
live object. Ask yourself what your template's `status.phrase` actually says,
and why.

### A7. Sync waves (15 min)

**Goal:** control ordering; we'll need it for "environment before app".

**Do:** in the playground, put the ConfigMap in wave `0` and the Deployment in
wave `1` with `metadata.annotations: argocd.argoproj.io/sync-wave: "1"`.
Delete the app's resources (`argocd app delete playground --cascade`),
re-create the app, and watch the sync in the UI.

**Notice:** Argo CD waits for every resource in a wave to be **Healthy**
before starting the next wave. So a correct Lua health check also makes
ordering correct.

### A8. A toy Config Management Plugin (30 min) 🛑

**Goal:** learn the sidecar mechanics with something trivial before adding
Bicep and `rad`.

The plugin turns every `*.txt` file into a ConfigMap.

**Do:**

1. Add to the values file:

   ```yaml
   configs:
     cmp:
       create: true
       plugins:
         txt2cm:
           discover:
             fileName: "*.txt"
           generate:
             command: [sh, -c]
             args:
               - |
                 for f in *.txt; do
                   n=$(basename "$f" .txt)
                   echo "---"
                   echo "apiVersion: v1"
                   echo "kind: ConfigMap"
                   echo "metadata: {name: $n}"
                   echo "data: {content: \"$(cat "$f")\", greeting: \"$PARAM_GREETING\"}"
                 done
   repoServer:
     extraContainers:
       - name: txt2cm
         image: busybox:1.36
         command: [/var/run/argocd/argocd-cmp-server]
         securityContext: {runAsNonRoot: true, runAsUser: 999}
         volumeMounts:
           - {mountPath: /var/run/argocd, name: var-files}
           - {mountPath: /home/argocd/cmp-server/plugins, name: plugins}
           - {mountPath: /home/argocd/cmp-server/config/plugin.yaml, subPath: txt2cm.yaml, name: argocd-cmp-cm}
           - {mountPath: /tmp, name: cmp-tmp}
     volumes:
       - {name: argocd-cmp-cm, configMap: {name: argocd-cmp-cm}}
       - {name: cmp-tmp, emptyDir: {}}
   ```

2. `helm upgrade ...` again, then confirm the repo-server now has two
   containers (the command from A1).
3. Add `txt/hello.txt` to your playground repo and create an Application with
   `path: txt` and:

   ```yaml
   source:
     plugin:
       name: txt2cm
       parameters:
         - name: greeting
           string: hi-from-argo
   ```

4. Break it: make the script `exit 1`, push, and look at the app's
   **ComparisonError**.

**Check:** a `hello` ConfigMap exists with your file content and the greeting.

**Notice:**

- The sidecar image only needs a shell; Argo CD injects `argocd-cmp-server`
  through the `var-files` volume.
- Parameters arrive as `PARAM_<NAME>`.
- stdout is the manifest; a non-zero exit is a ComparisonError.
- The rendered output is visible in the UI's **App Details → Manifest**. That
  is where a secure parameter would leak.

---

## Part B — Guided prototype: the Radius plugin (about 3–4 hours)

Same pattern as A8, but the plugin runs `rad bicep generate-kubernetes-manifest`
and Radius's controller does the deploying.

### B1. Run the pipeline by hand first (30 min) 🛑

**Goal:** understand exactly what the plugin will automate.

**Do:**

```bash
mkdir -p ~/argo-proto && cd ~/argo-proto
cp /Users/nithya/ignite2026/environments/kubernetes.bicep /Users/nithya/ignite2026/bicepconfig.json .
rad bicep generate-kubernetes-manifest kubernetes.bicep --group voting-k8s --destination-file env.yaml
less env.yaml            # a DeploymentTemplate: spec.template is compiled ARM JSON
kubectl create namespace voting-k8s
kubectl apply -n voting-k8s -f env.yaml
kubectl -n voting-k8s get deploymenttemplate -w     # watch status.phrase
kubectl -n voting-k8s get deploymentresources
```

**Notice:**

- the shape of `spec.template`, `spec.parameters` and `spec.providerConfig`,
- how `status.phrase` moves,
- that `DeploymentResource` objects appear, one per Radius resource.

Then delete it (`kubectl delete -f env.yaml`) and watch Radius clean up.

### B2. Make the sample repo (20 min)

**Goal:** a Git repo holding only the Kubernetes parts of `ignite2026`.

**Do:** create `nithyatsu/ignite2026-argocd` (public) with:

- `bicepconfig.json` (as is),
- `environments/kubernetes.bicep` (as is),
- `app/app.bicep`: remove `platformOptions`, and default `imageTag` to
  `047c81d1f93c53cad6074db9ad525bf37a541178-arm64` and `redisImage` to the
  arm64 digest in `.env.local`.

Decision for you: one folder per Application (`environments/`, `app/`),
because the plugin compiles one Bicep file per Application. Where does
`bicepconfig.json` go so both folders find it? (Hint: Bicep searches parent
folders, but Argo CD only gives the plugin the app's path. Look up
**Application source `path` vs. repo root** in the CMP docs.)

### B3. Build the sidecar image (40 min) 🛑

**Goal:** a container with `rad` and Bicep that runs as user 999.

**Do:** write a Dockerfile. I'll help you with:

- installing `rad` 0.61.1 for linux/arm64,
- pre-downloading the Bicep binary, because `rad` fetches it on first use,
- where `HOME` must point when running as user 999.

Then:

```bash
docker build -t radius-cmp:dev .
docker run --rm -it --user 999 -v ~/argo-proto:/w -w /w radius-cmp:dev \
  sh -c 'rad bicep generate-kubernetes-manifest kubernetes.bicep -g voting-k8s -d /tmp/o.yaml && head /tmp/o.yaml'
kind load docker-image radius-cmp:dev --name kind-argo
```

**Notice:** whether the container can reach `biceptypes.azurecr.io` to pull
the Radius types. That's a real requirement for Argo CD admins.

### B4. Register the plugin (30 min)

**Goal:** replace the toy plugin with `radius-bicep`.

**Do:** mirror A8:

- an extra container using `radius-cmp:dev`,
- `generate` writes to a temp file and prints it, because the released CLI
  can't write to stdout,
- `bicepFile`, `resourceGroup` and `postgresPassword` parameters.

Use the Lua health check from A6, extended to check
`observedGeneration == metadata.generation`, as in the spec's health check
sketch.

### B5. Environment Application (20 min) 🛑

**Do:** create Application `voting-env` (path `environments`, plugin
`radius-bicep`, destination namespace `voting-k8s`, `CreateNamespace=true`).

**Check:** the `DeploymentTemplate` goes Progressing → Healthy.

**Decision for you:** how the app waits for the environment:

- two Applications created in order,
- app-of-apps with sync waves (A7), or
- one Application.

### B6. App Application and the secret (30 min) 🛑

**Do:** create Application `voting-app` (path `app`), passing
`environment` and `postgresPassword` as plugin parameters.

**Check:** vote and result Running in `voting-k8s`.

**Notice:** open **App Details → Manifest** and **Diff** and find the
password. Write down every place it appears; this is spec gap 1.

### B7. Scenarios (60 min)

For each one, write down what Argo CD shows, what Radius shows, and whether it
matches the spec.

| # | Scenario                        | Do                                                            | Spec section to compare |
|---|---------------------------------|---------------------------------------------------------------|-------------------------|
| 1 | Parameter-only change           | Change `imageTag` (or another parameter); push                | User story 3            |
| 2 | Bicep compile error             | Break `app.bicep` syntax; push                                | Error handling          |
| 3 | Radius deployment failure       | Point a recipe at a bad tag; push                             | Health mapping          |
| 4 | Refresh vs. retry after failure | Fix nothing, click Refresh, then Sync                         | Retry open question     |
| 5 | Remove a resource               | Delete `resultContainer` from Bicep; push                     | Pending removals        |
| 6 | Delete the Application          | `argocd app delete voting-app --cascade`                      | Deletion open question  |
| 7 | Wrong order                     | Create the app before the environment                         | No-retry behavior       |
| 8 | What the tree shows             | Compare the Argo CD tree with `kubectl -n voting-k8s get all` | User story 4            |

### B8. Write it up (30 min)

Fold the findings into the spec (PR #13323): confirmed claims, wrong claims,
and new gaps (starting with secure parameters).

---

## Cleanup (when finished)

```bash
argocd app delete voting-app voting-env playground guestbook --cascade -y
kind delete cluster --name kind-argo
```

---

## Learnings

Argo CD is a pull-based GitOps controller. CD stands for continuous delivery, and an Argo CD Application ties a Git repository, revision and path to a destination cluster and namespace. The repo-server renders the manifests, and the application-controller compares them with the live objects and applies them. Plain YAML folders, Helm and Kustomize are built in; anything else needs a Config Management Plugin (CMP) running as a sidecar on the repo-server. That sidecar is how Radius would compile Bicep.

Sync status (Synced or OutOfSync) and health (Healthy, Progressing, Degraded, Missing) are separate. Refresh and hard refresh only compare (hard refresh also skips the render cache); sync applies. Automated sync applies new commits, self-heal reverts manual changes in the cluster, and prune deletes objects that were removed from Git. Argo CD polls Git about every three minutes and notices cluster drift through watches.

In the Radius design, Argo CD never talks to Radius. The plugin turns Bicep into a `DeploymentTemplate` custom resource, which holds the compiled ARM JSON as one string. Argo CD applies it and the Radius controller deploys it, so the `DeploymentTemplate` is the only thing the two share.

Radius still does all the real deploying, just not through `rad deploy`. Argo CD's apply only puts the `DeploymentTemplate` object into the cluster; Synced means that object matches Git. The Radius controller watches these objects and sends the ARM JSON to the same place `rad deploy` does: an incremental ARM deployment in UCP, run by the deployment engine, which creates the real resources through Recipes. The controller then writes `status.phrase` and `status.outputResources` back onto the object, and the Lua health check turns that into Argo CD health. The controller stores a hash of the spec in `status.statusHash` and deploys again only when the spec changes, deleting resources removed from the template on the way. It does not notice changes to the resources it created. So Argo CD keeps the `DeploymentTemplate` in line with Git, and Radius keeps the real resources in line with the `DeploymentTemplate`, but only when the template changes.

Trying this by hand exposed gaps in the reused CRD and controller. The object is named after the Bicep file. An empty `parameters` map is dropped when the controller adds its finalizer, so the app stays OutOfSync forever. Secrets would sit in plain text. The single large string makes diffs hard to read and can exceed the annotation size limit. Drift in what Radius created isn't fixed.

Argo CD doesn't know the health of a custom resource, so it needs a Lua health check that reads `status.phrase`. `Ready` means only that provisioning finished, not that the application is running. Will's feature spec wants failures after provisioning to show in health where Radius can observe them, which points to Radius watching what it created and writing that into the `DeploymentTemplate` status. All of this is logged in PR #13323 (open questions 7 and 10, and the naming decision) and in the prototype log.

A health check is installed through the Helm values: every key under `configs.cm` becomes a key in the `argocd-cm` ConfigMap, which the controller reads without a restart. The key is `resource.customizations.health.<group>_<Kind>`, taken from the object's `apiVersion` without the version, so ours is `radapp.io_DeploymentTemplate`. The check is a small Lua script that sees only the live object as `obj` and returns a status and message. After installing it, `empty.bicep` showed Healthy, because an empty template has nothing to deploy and goes straight to `Ready`.

Sync waves are an annotation, `argocd.argoproj.io/sync-wave`, that orders objects inside one Application; Argo CD waits for a wave to be Healthy before the next, so waves rely on the health check. Inside one Bicep file Radius already orders resources, because a symbolic reference such as `recipePacks: [ recipepack.id ]` becomes a `dependsOn`. Between files nothing links them: each file is its own `DeploymentTemplate`, and the app gets the environment as a plain string, so pack, environment and app need an outside order. The generator cannot add annotations, so waves would have to come from the plugin, a plugin parameter, a new generator flag, or child Applications in an app-of-apps. Creating the Applications in order, or making the reconciler retry until the environment exists, avoids waves; this is logged as the open ordering decision in the prototype log.
