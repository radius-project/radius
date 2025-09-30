#!/usr/bin/env bash
set -euo pipefail

NAMESPACE=${NAMESPACE:-githttpbackend}
USERNAME=${USERNAME:-testuser}
PASSWORD=${PASSWORD:-testpass}
IMAGE=${IMAGE:-ghcr.io/willdavsmith/radius/githttpbackend:latest}
REPO_NAME=${REPO_NAME:-manual-test-repo}

echo "Installing Git HTTP backend (${IMAGE}) into namespace ${NAMESPACE}..."
./.github/actions/install-git-http-backend/install-git-http-backend.sh \
  "${USERNAME}" "${PASSWORD}" "${NAMESPACE}" "${IMAGE}"

kubectl rollout status deployment/githttpbackend -n "${NAMESPACE}" --timeout=180s

echo "Port-forwarding service git-http on 30080 -> 3000"
kubectl port-forward -n "${NAMESPACE}" svc/git-http 30080:3000 >/tmp/git-http-forward.log 2>&1 &
PF_PID=$!

WORKTREEDIR="$(mktemp -d)"
trap 'kill ${PF_PID} 2>/dev/null || true; rm -rf "${WORKTREEDIR}"' EXIT

sleep 5
status=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:30080/ || true)
if [[ -z "${status}" || "${status}" == "000" ]]; then
  echo "Port-forward failed"
  exit 1
fi

# Ensure bare repo exists on the git backend
echo "Initializing bare repository ${REPO_NAME}.git on the server"
kubectl exec -n "${NAMESPACE}" deploy/githttpbackend -- \
  sh -c "set -e; cd /var/lib/git; rm -rf ${REPO_NAME}.git; git init --bare ${REPO_NAME}.git >/dev/null; cd ${REPO_NAME}.git; git symbolic-ref HEAD refs/heads/main; git config http.receivepack true; git config http.uploadpack true; git update-server-info; chmod -R 775 ."

pushd "${WORKTREEDIR}" >/dev/null
git init
echo "hello" > README.md
git add README.md
git -c user.name="${USERNAME}" -c user.email="${USERNAME}@example.com" commit -m "init"
git branch -M main
git remote add origin "http://${USERNAME}:${PASSWORD}@localhost:30080/${REPO_NAME}.git"
git push --set-upstream origin main

echo "Confirming remote repository is reachable"
status=$(curl -s -o /dev/null -w "%{http_code}" -u "${USERNAME}:${PASSWORD}" \
  "http://localhost:30080/${REPO_NAME}.git/info/refs?service=git-upload-pack" || true)
if [[ "${status}" != "200" ]]; then
  echo "Unexpected status when contacting remote repo (info/refs): ${status}"
  exit 1
fi

echo "Cloning back the repo to verify..."
git clone "http://${USERNAME}:${PASSWORD}@localhost:30080/${REPO_NAME}.git" clone-test
ls clone-test

popd >/dev/null
echo "All good! (Repo living at http://${USERNAME}:${PASSWORD}@localhost:30080/${REPO_NAME}.git)"
