# Build SPA
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
# npm install (not ci): lock may omit optional platform packages resolved only on linux
RUN npm install --no-audit --no-fund
COPY . .
# Empty string = same-origin relative /api (required for production domains / Coolify HTTPS).
# Local compose can still override with http://localhost:27341 if needed.
ARG VITE_API_BASE_URL=
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
RUN npm run build

# Serve static files
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
# Coolify / reverse proxies expect container port 80 by default
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
