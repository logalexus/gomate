FROM golang:1.27.4 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY bench/mock/ ./bench/mock/
RUN CGO_ENABLED=0 go build -trimpath -o /mock ./bench/mock
FROM scratch
COPY --from=build /mock /mock
USER 65532:65532
ENTRYPOINT ["/mock"]
