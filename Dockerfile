# Cross-compiles on the build host for each target platform, so multi-arch
# builds (amd64, arm64, arm/v7) don't run the Go toolchain under emulation.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH TARGETVARIANT
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath -ldflags="-s -w" -o /airgradient2mqtt

FROM scratch
COPY --from=build /airgradient2mqtt /airgradient2mqtt
CMD ["/airgradient2mqtt", "-config", "/config.toml"]
