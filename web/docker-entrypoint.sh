#!/bin/sh
set -e
# Default: API on the compose network. Override with API_UPSTREAM=host.docker.internal:27341
# when the Go API runs on the Docker host (required for reliable Tailscale 100.x SSH
# from Docker Desktop on macOS).
: "${API_UPSTREAM:=api:27341}"
export API_UPSTREAM
envsubst '${API_UPSTREAM}' < /etc/nginx/templates/default.conf.template \
  > /etc/nginx/conf.d/default.conf
exec nginx -g 'daemon off;'
