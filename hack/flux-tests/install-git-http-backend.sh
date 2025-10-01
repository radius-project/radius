#!/usr/bin/env bash
# Installs a Git HTTP backend backed by alpine/git and nginx.
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <git-username> <git-password> [namespace] [image]" >&2
  exit 1
fi

GIT_USERNAME=$1
GIT_PASSWORD=$2
NAMESPACE=${3:-githttpbackend}
IMAGE=${4:-${GIT_HTTP_IMAGE:-alpine/git:2.45.2}} # allow override via env or arg
SERVER_TEMP_DIR=${GIT_SERVER_TEMP_DIR:-/var/lib/git}

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

kubectl -n "${NAMESPACE}" create secret generic githttpbackend-auth \
  --type=kubernetes.io/basic-auth \
  --from-literal=username="${GIT_USERNAME}" \
  --from-literal=password="${GIT_PASSWORD}" \
  --dry-run=client -o yaml | kubectl apply -f -

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
          command:
            - /bin/sh
            - -c
            - |
              set -euo pipefail
              apk add --no-cache nginx fcgiwrap spawn-fcgi apache2-utils

              htpasswd -bc /etc/nginx/.htpasswd "\${GIT_USERNAME}" "\${GIT_PASSWORD}"

              DOLLAR='$'
              SOCKET_DIR="/run/git-http"

              {
                  printf 'server {\n'
                  printf '    listen 3000;\n'
                  printf '    server_name _;\n'
                  printf '    root %s;\n\n' "${SERVER_TEMP_DIR}"
                  printf '    auth_basic "Git Server";\n'
                  printf '    auth_basic_user_file /etc/nginx/.htpasswd;\n\n'
                  printf '    location / {\n'
                  printf '        client_max_body_size 0;\n'
                  printf '        include fastcgi_params;\n'
                  printf '        fastcgi_pass unix:%s/fcgiwrap.sock;\n' "\${SOCKET_DIR}"
                  printf '        fastcgi_param SCRIPT_FILENAME /usr/libexec/git-core/git-http-backend;\n'
                  printf '        fastcgi_param PATH_INFO %suri;\n' "\${DOLLAR}"
                  printf '        fastcgi_param SCRIPT_NAME %suri;\n' "\${DOLLAR}"
                  printf '        fastcgi_param GIT_PROJECT_ROOT %s;\n' "${SERVER_TEMP_DIR}"
                  printf '        fastcgi_param GIT_HTTP_EXPORT_ALL "";\n'
                  printf '        fastcgi_param GIT_HTTP_MAX_REQUEST_BUFFER 1048576000;\n'
                  printf '        fastcgi_param REMOTE_USER %sremote_user;\n' "\${DOLLAR}"
                  printf '        fastcgi_param HTTP_AUTHORIZATION %shttp_authorization;\n' "\${DOLLAR}"
                  printf '    }\n'
                  printf '}\n'
              } >/etc/nginx/http.d/git.conf

              mkdir -p "\${SOCKET_DIR}" "${SERVER_TEMP_DIR}"
              chown -R nginx:nginx "\${SOCKET_DIR}" "${SERVER_TEMP_DIR}"

              spawn-fcgi -s "\${SOCKET_DIR}/fcgiwrap.sock" -M 766 -u nginx -g nginx /usr/bin/fcgiwrap
              exec nginx -g 'daemon off;'
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

kubectl rollout status deployment/githttpbackend -n "${NAMESPACE}" --timeout=240s
