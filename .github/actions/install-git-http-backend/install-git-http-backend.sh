#!/bin/bash

set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "Usage: install-githttpbackend.sh <git-username> <git-password> [namespace]"
  exit 1
fi

GIT_USERNAME="$1"
GIT_PASSWORD="$2"
NAMESPACE="${3:-githttpbackend}"
IMAGE="${4:-ghcr.io/radius-project/githttpbackend:latest}"
SERVER_TEMP_DIR="/var/lib/git"

# Ensure namespace exists
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# Create or update auth secret
kubectl -n "$NAMESPACE" create secret generic githttpbackend-auth \
  --type=kubernetes.io/basic-auth \
  --from-literal=username="$GIT_USERNAME" \
  --from-literal=password="$GIT_PASSWORD" \
  --dry-run=client -o yaml | kubectl apply -f -

# Create or update service account (optional for future RBAC flexibility)
cat <<YAML | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: githttpbackend
  namespace: ${NAMESPACE}
  labels:
    app: githttpbackend
YAML

# Create or update deployment and service
cat <<YAML | kubectl apply -f -
apiVersion: apps/v1
kind: Deployment
metadata:
  name: githttpbackend
  namespace: ${NAMESPACE}
  labels:
    app: githttpbackend
spec:
  replicas: 1
  selector:
    matchLabels:
      app: githttpbackend
  template:
    metadata:
      labels:
        app: githttpbackend
    spec:
      serviceAccountName: githttpbackend
      containers:
        - name: githttpbackend
          image: ${IMAGE}
          imagePullPolicy: IfNotPresent
          env:
            - name: GIT_USERNAME
              valueFrom:
                secretKeyRef:
                  name: githttpbackend-auth
                  key: username
            - name: GIT_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: githttpbackend-auth
                  key: password
            - name: GIT_SERVER_TEMP_DIR
              value: ${SERVER_TEMP_DIR}
            - name: GIT_REPO_NAME
              value: seed-repo
          ports:
            - containerPort: 3000
              name: http
          readinessProbe:
            tcpSocket:
              port: 3000
            initialDelaySeconds: 5
            periodSeconds: 5
          livenessProbe:
            tcpSocket:
              port: 3000
            initialDelaySeconds: 10
            periodSeconds: 10
          volumeMounts:
            - name: git-data
              mountPath: ${SERVER_TEMP_DIR}
      volumes:
        - name: git-data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: git-http
  namespace: ${NAMESPACE}
  labels:
    app: githttpbackend
spec:
  selector:
    app: githttpbackend
  ports:
    - name: http
      port: 3000
      targetPort: 3000
      protocol: TCP
YAML

# Wait for deployment to be ready
kubectl rollout status deployment/githttpbackend -n "$NAMESPACE" --timeout=240s
