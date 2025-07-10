# syntax = docker/dockerfile-upstream:1.16.0-labs

# THIS FILE WAS AUTOMATICALLY GENERATED, PLEASE DO NOT EDIT.
#
# Generated on 2025-07-10T13:39:43Z by kres 1700045.

ARG TOOLCHAIN

FROM ghcr.io/siderolabs/ca-certificates:v1.10.0 AS image-ca-certificates

FROM ghcr.io/siderolabs/fhs:v1.10.0 AS image-fhs

FROM ghcr.io/siderolabs/ipxe:v1.11.0-alpha.0-37-gfadf1e2 AS ipxe

FROM --platform=linux/amd64 ghcr.io/siderolabs/ipxe:v1.11.0-alpha.0-37-gfadf1e2 AS ipxe-linux-amd64

FROM --platform=linux/arm64 ghcr.io/siderolabs/ipxe:v1.11.0-alpha.0-37-gfadf1e2 AS ipxe-linux-arm64

FROM ghcr.io/siderolabs/liblzma:v1.11.0-alpha.0-37-gfadf1e2 AS liblzma

# runs markdownlint
FROM docker.io/oven/bun:1.2.15-alpine AS lint-markdown
WORKDIR /src
RUN bun i markdownlint-cli@0.45.0 sentences-per-line@0.3.0
COPY .markdownlint.json .
COPY ./README.md ./README.md
RUN bunx markdownlint --ignore "CHANGELOG.md" --ignore "**/node_modules/**" --ignore '**/hack/chglog/**' --rules sentences-per-line .

FROM ghcr.io/siderolabs/musl:v1.11.0-alpha.0-37-gfadf1e2 AS musl

# base toolchain image
FROM --platform=${BUILDPLATFORM} ${TOOLCHAIN} AS toolchain
RUN apk --update --no-cache add bash curl build-base protoc protobuf-dev

# build tools
FROM --platform=${BUILDPLATFORM} toolchain AS tools
ENV GO111MODULE=on
ARG CGO_ENABLED
ENV CGO_ENABLED=${CGO_ENABLED}
ARG GOTOOLCHAIN
ENV GOTOOLCHAIN=${GOTOOLCHAIN}
ARG GOEXPERIMENT
ENV GOEXPERIMENT=${GOEXPERIMENT}
ENV GOPATH=/go
ARG DEEPCOPY_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build,id=dhcp-proxy-ipxe/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg go install github.com/siderolabs/deep-copy@${DEEPCOPY_VERSION} \
	&& mv /go/bin/deep-copy /bin/deep-copy
ARG GOLANGCILINT_VERSION
RUN --mount=type=cache,target=/root/.cache/go-build,id=dhcp-proxy-ipxe/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCILINT_VERSION} \
	&& mv /go/bin/golangci-lint /bin/golangci-lint
RUN --mount=type=cache,target=/root/.cache/go-build,id=dhcp-proxy-ipxe/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg go install golang.org/x/vuln/cmd/govulncheck@latest \
	&& mv /go/bin/govulncheck /bin/govulncheck
ARG GOFUMPT_VERSION
RUN go install mvdan.cc/gofumpt@${GOFUMPT_VERSION} \
	&& mv /go/bin/gofumpt /bin/gofumpt

# tools and sources
FROM tools AS base
WORKDIR /src
COPY go.mod go.mod
COPY go.sum go.sum
RUN cd .
RUN --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg go mod download
RUN --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg go mod verify
COPY ./cmd ./cmd
COPY ./internal ./internal
RUN --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg go list -mod=readonly all >/dev/null

FROM tools AS embed-generate
ARG SHA
ARG TAG
WORKDIR /src
RUN mkdir -p internal/version/data && \
    echo -n ${SHA} > internal/version/data/sha && \
    echo -n ${TAG} > internal/version/data/tag

# runs gofumpt
FROM base AS lint-gofumpt
RUN FILES="$(gofumpt -l .)" && test -z "${FILES}" || (echo -e "Source code is not formatted with 'gofumpt -w .':\n${FILES}"; exit 1)

# runs golangci-lint
FROM base AS lint-golangci-lint
WORKDIR /src
COPY .golangci.yml .
ENV GOGC=50
RUN --mount=type=cache,target=/root/.cache/go-build,id=dhcp-proxy-ipxe/root/.cache/go-build --mount=type=cache,target=/root/.cache/golangci-lint,id=dhcp-proxy-ipxe/root/.cache/golangci-lint,sharing=locked --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg golangci-lint run --config .golangci.yml

# runs govulncheck
FROM base AS lint-govulncheck
WORKDIR /src
RUN --mount=type=cache,target=/root/.cache/go-build,id=dhcp-proxy-ipxe/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg govulncheck ./...

# runs unit-tests with race detector
FROM base AS unit-tests-race
WORKDIR /src
ARG TESTPKGS
RUN --mount=type=cache,target=/root/.cache/go-build,id=dhcp-proxy-ipxe/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg --mount=type=cache,target=/tmp,id=dhcp-proxy-ipxe/tmp CGO_ENABLED=1 go test -race ${TESTPKGS}

# runs unit-tests
FROM base AS unit-tests-run
WORKDIR /src
ARG TESTPKGS
RUN --mount=type=cache,target=/root/.cache/go-build,id=dhcp-proxy-ipxe/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg --mount=type=cache,target=/tmp,id=dhcp-proxy-ipxe/tmp go test -covermode=atomic -coverprofile=coverage.txt -coverpkg=${TESTPKGS} ${TESTPKGS}

FROM embed-generate AS embed-abbrev-generate
WORKDIR /src
ARG ABBREV_TAG
RUN echo -n 'undefined' > internal/version/data/sha && \
    echo -n ${ABBREV_TAG} > internal/version/data/tag

FROM scratch AS unit-tests
COPY --from=unit-tests-run /src/coverage.txt /coverage-unit-tests.txt

# cleaned up specs and compiled versions
FROM scratch AS generate
COPY --from=embed-abbrev-generate /src/internal/version internal/version

# builds dhcp-proxy-ipxe-linux-amd64
FROM base AS dhcp-proxy-ipxe-linux-amd64-build
COPY --from=generate / /
COPY --from=embed-generate / /
WORKDIR /src/cmd/dhcp-proxy-ipxe
ARG GO_BUILDFLAGS
ARG GO_LDFLAGS
ARG VERSION_PKG="internal/version"
ARG SHA
ARG TAG
RUN --mount=type=cache,target=/root/.cache/go-build,id=dhcp-proxy-ipxe/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg GOARCH=amd64 GOOS=linux go build ${GO_BUILDFLAGS} -ldflags "${GO_LDFLAGS} -X ${VERSION_PKG}.Name=dhcp-proxy-ipxe -X ${VERSION_PKG}.SHA=${SHA} -X ${VERSION_PKG}.Tag=${TAG}" -o /dhcp-proxy-ipxe-linux-amd64

# builds dhcp-proxy-ipxe-linux-arm64
FROM base AS dhcp-proxy-ipxe-linux-arm64-build
COPY --from=generate / /
COPY --from=embed-generate / /
WORKDIR /src/cmd/dhcp-proxy-ipxe
ARG GO_BUILDFLAGS
ARG GO_LDFLAGS
ARG VERSION_PKG="internal/version"
ARG SHA
ARG TAG
RUN --mount=type=cache,target=/root/.cache/go-build,id=dhcp-proxy-ipxe/root/.cache/go-build --mount=type=cache,target=/go/pkg,id=dhcp-proxy-ipxe/go/pkg GOARCH=arm64 GOOS=linux go build ${GO_BUILDFLAGS} -ldflags "${GO_LDFLAGS} -X ${VERSION_PKG}.Name=dhcp-proxy-ipxe -X ${VERSION_PKG}.SHA=${SHA} -X ${VERSION_PKG}.Tag=${TAG}" -o /dhcp-proxy-ipxe-linux-arm64

FROM scratch AS dhcp-proxy-ipxe-linux-amd64
COPY --from=dhcp-proxy-ipxe-linux-amd64-build /dhcp-proxy-ipxe-linux-amd64 /dhcp-proxy-ipxe-linux-amd64

FROM scratch AS dhcp-proxy-ipxe-linux-arm64
COPY --from=dhcp-proxy-ipxe-linux-arm64-build /dhcp-proxy-ipxe-linux-arm64 /dhcp-proxy-ipxe-linux-arm64

FROM dhcp-proxy-ipxe-linux-${TARGETARCH} AS dhcp-proxy-ipxe

FROM scratch AS dhcp-proxy-ipxe-all
COPY --from=dhcp-proxy-ipxe-linux-amd64 / /
COPY --from=dhcp-proxy-ipxe-linux-arm64 / /

FROM scratch AS image-dhcp-proxy-ipxe
ARG TARGETARCH
COPY --from=dhcp-proxy-ipxe dhcp-proxy-ipxe-linux-${TARGETARCH} /dhcp-proxy-ipxe
COPY --from=image-fhs / /
COPY --from=image-ca-certificates / /
COPY --from=musl / /
COPY --from=liblzma / /
COPY --from=ipxe /usr/libexec/zbin /usr/bin/zbin
COPY --from=ipxe-linux-amd64 /usr/libexec/ /var/lib/ipxe/amd64
COPY --from=ipxe-linux-arm64 /usr/libexec/ /var/lib/ipxe/arm64
LABEL org.opencontainers.image.source=https://github.com/siderolabs/dhcp-proxy-ipxe
ENTRYPOINT ["/dhcp-proxy-ipxe"]

