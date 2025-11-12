#!/bin/sh
set -eu

# Ensure PATH_INFO always starts with a leading slash to keep git-http-backend happy.
if [ "${PATH_INFO:-}" != "" ] && [ "${PATH_INFO#*/}" = "${PATH_INFO}" ]; then
  PATH_INFO="/${PATH_INFO}"
  export PATH_INFO
fi

# Default to exporting all repositories unless explicitly disabled.
export GIT_HTTP_EXPORT_ALL=${GIT_HTTP_EXPORT_ALL:-1}

exec /usr/libexec/git-core/git-http-backend
