# Build SPA
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
# npm install (not ci): lock may omit optional platform packages resolved only on linux
RUN npm install --no-audit --no-fund
COPY . .
# Empty base = same-origin relative /api when behind reverse proxy;
# for compose we call the api service hostname from the browser via published port.
ARG VITE_API_BASE_URL=http://localhost:8080
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
RUN npm run build

# Serve static files
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
