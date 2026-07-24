# Build
FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/agent-hub ./cmd/agent-hub

# Run
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget \
  && adduser -D -H -u 10001 app \
  && mkdir -p /data \
  && chown app:app /data
USER app
WORKDIR /app
COPY --from=build /out/agent-hub /app/agent-hub
# Uncommon default to avoid Coolify / multi-app host port clashes
ENV HTTP_ADDR=:27341
ENV DATA_DIR=/data
EXPOSE 27341
ENTRYPOINT ["/app/agent-hub"]
