FROM node:22-bookworm-slim AS dashboard
WORKDIR /src/dashboard
RUN corepack enable && corepack prepare pnpm@9.15.9 --activate
COPY dashboard/package.json dashboard/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY dashboard/ ./
COPY VERSION ../VERSION
RUN pnpm build

FROM scratch AS dashboard-artifact
COPY --from=dashboard /src/dashboard/dist/ /

FROM golang:1.26-bookworm AS backend
ARG APP_VERSION
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
RUN test -n "$APP_VERSION" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X github.com/verdantflarehub/verdantflare-app-project/internal/buildinfo.Version=$APP_VERSION" -o /project ./cmd/project

FROM gcr.io/distroless/static-debian12:nonroot
ARG APP_VERSION
LABEL org.opencontainers.image.version=$APP_VERSION
WORKDIR /app
COPY --from=backend /project /project
COPY --from=dashboard /src/dashboard/dist/ dashboard/dist/
ENV PROJECT_LISTEN=0.0.0.0:8443 PROJECT_DASHBOARD_DIR=/app/dashboard/dist
EXPOSE 8443
ENTRYPOINT ["/project"]
