# Build SPA
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
# npm ci: reproducible, and it fails loudly when the lockfile drifts from
# package.json. The lockfile must be regenerated on linux (see CONTRIBUTING.md)
# — one written on macOS omitted linux-only optional packages and broke this.
RUN npm ci --no-audit --no-fund
COPY . .
# Always bake empty API base so Coolify never injects localhost into the SPA.
# Runtime also strips localhost (see src/lib/api.ts). Same-origin /api via nginx.
ARG VITE_API_BASE_URL=
ENV VITE_API_BASE_URL=
RUN npm run build

# Serve static files
FROM nginx:1.27-alpine
# envsubst for API_UPSTREAM (compose service vs host Tailscale path)
RUN apk add --no-cache gettext
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY docker-entrypoint.sh /docker-entrypoint-agent-hub.sh
RUN chmod +x /docker-entrypoint-agent-hub.sh
COPY --from=build /app/dist /usr/share/nginx/html
# Coolify / reverse proxies expect container port 80 by default
EXPOSE 80
ENV API_UPSTREAM=api:27341
ENTRYPOINT ["/docker-entrypoint-agent-hub.sh"]
