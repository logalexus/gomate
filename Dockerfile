FROM node:20-alpine AS web
WORKDIR /src
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27.4 AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -o /packmate ./cmd/packmate
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -o /import-legacy ./cmd/import-legacy
RUN --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -o /discoverer ./cmd/discoverer

FROM scratch
COPY --from=go /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=go /packmate /packmate
COPY --from=go /import-legacy /import-legacy
COPY --from=go /discoverer /discoverer
COPY --from=web /src/dist /web
ENTRYPOINT ["/packmate"]
