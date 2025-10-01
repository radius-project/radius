#!/bin/sh
# Bootstrap script for the alpine/git container. Installs nginx/fcgiwrap, renders configuration from
# templates, and launches the Git HTTP backend suitable for Radius functional tests.
set -eu

# Ensure nginx + fcgiwrap + git HTTP backend bits are present (alpine/git is minimal by default).
apk add --no-cache nginx fcgiwrap spawn-fcgi apache2-utils gettext git-daemon

# Generate the basic-auth credentials picked up by nginx.
htpasswd -bc /etc/nginx/.htpasswd "${GIT_USERNAME}" "${GIT_PASSWORD}"

# fcgiwrap needs a UNIX socket; keep everything under /run/git-http.
SOCKET_DIR="/run/git-http"
export SOCKET_DIR

# Prepare writable directories for the socket and bare Git repos.
mkdir -p "${SOCKET_DIR}" "${GIT_SERVER_TEMP_DIR}"
chown -R nginx:nginx "${SOCKET_DIR}" "${GIT_SERVER_TEMP_DIR}"

# Allow nginx user to interact with repositories regardless of owner checks.
git config --system --add safe.directory "${GIT_SERVER_TEMP_DIR}"
git config --system --add safe.directory "${GIT_SERVER_TEMP_DIR}/*"

# Render the nginx configuration using the environment exported above.
envsubst '${GIT_SERVER_TEMP_DIR} ${SOCKET_DIR}' </config/nginx.conf.template >/etc/nginx/http.d/git.conf

# Start fcgiwrap (serves /usr/libexec/git-core/git-http-backend) and foreground nginx.
spawn-fcgi -s "${SOCKET_DIR}/fcgiwrap.sock" -M 766 -u nginx -g nginx /usr/bin/fcgiwrap
exec nginx -g 'daemon off;'
