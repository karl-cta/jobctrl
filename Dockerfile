FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

FROM golang:1.26-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o job-ctrl .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata curl
WORKDIR /app
COPY --from=go-builder /app/job-ctrl .

LABEL org.opencontainers.image.source="https://github.com/karl-cta/jobctrl" \
      org.opencontainers.image.description="Self-hosted job application tracker" \
      org.opencontainers.image.licenses="MIT"

ENV JOB_CTRL_DB_PATH=/data/job-ctrl.db
ENV JOB_CTRL_ADDR=:8080
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD curl -f http://localhost:8080/api/health || exit 1
ENTRYPOINT ["./job-ctrl"]
