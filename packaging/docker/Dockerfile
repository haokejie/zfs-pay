# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.24-bookworm AS build
ARG TARGETARCH
ARG VERSION
ARG COMMIT=unknown
ARG BUILD_DATE=1970-01-01T00:00:00Z
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH="$TARGETARCH" go build -trimpath \
      -ldflags "-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.buildDate=$BUILD_DATE" \
      -o /out/zfs-pay ./cmd/zfs-pay && \
    CGO_ENABLED=0 GOOS=linux GOARCH="$TARGETARCH" go build -trimpath \
      -ldflags "-s -w" -o /out/zfs-pay-zed ./cmd/zfs-pay-zed

FROM --platform=$BUILDPLATFORM debian:bookworm-slim AS package
RUN apt-get update && apt-get install -y --no-install-recommends file && rm -rf /var/lib/apt/lists/*
ARG TARGETARCH
ARG VERSION
ARG SOURCE_DATE_EPOCH=0
WORKDIR /src
COPY --from=build /out /out/bin
COPY LICENSE README.md ./
COPY docs ./docs
COPY packaging ./packaging
COPY scripts/package/build-deb.sh scripts/package/verify-deb.sh ./scripts/package/
RUN ARCH="$TARGETARCH" VERSION="$VERSION" BINARY=/out/bin/zfs-pay HELPER=/out/bin/zfs-pay-zed \
      OUT_DIR=/out/packages SOURCE_DATE_EPOCH="$SOURCE_DATE_EPOCH" ./scripts/package/build-deb.sh && \
    ./scripts/package/verify-deb.sh /out/packages/zfs-pay_"$VERSION"_"$TARGETARCH".deb

FROM scratch AS artifact
COPY --from=package /out/packages/ /
