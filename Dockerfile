# syntax=docker/dockerfile:1

# Build on the host architecture and cross-compile to the target. Go does this
# natively, whereas emulating an arm64 toolchain under QEMU is an order of
# magnitude slower and occasionally miscompiles.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

WORKDIR /src

# Dependencies first: this layer stays cached until go.mod changes.
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

# CGO_ENABLED=0 produces a static binary, which is what lets the final stage
# carry no libc. -trimpath keeps build paths out of the binary: smaller, and
# one less detail leaked by a stack trace.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/bookplate ./cmd/bookplate

# distroless/static carries CA certificates and nothing else: no shell, no
# package manager, no curl. After a breakout there is no tooling to reuse.
FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev

LABEL org.opencontainers.image.title="BookPlate" \
      org.opencontainers.image.description="Self-hosted catalogue for the physical books in your home" \
      org.opencontainers.image.source="https://github.com/aydam2728/bookplate" \
      org.opencontainers.image.licenses="AGPL-3.0-or-later" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=build /out/bookplate /bookplate

# /data holds the database and the cover cache. It is the only path the server
# writes to, which is what makes a read-only root filesystem possible.
ENV BOOKPLATE_ADDR=":8080" \
    BOOKPLATE_DATA_DIR="/data"

EXPOSE 8080
USER nonroot:nonroot

# The binary probes itself, so the image needs no shell to be health-checked.
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/bookplate", "healthcheck"]

ENTRYPOINT ["/bookplate"]
