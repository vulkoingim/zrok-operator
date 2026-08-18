# Build the manager binary
FROM docker.io/golang:1.26 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG GIT_REVISION=none
ARG DATE=unknown

WORKDIR /workspace
COPY go.mod go.mod
COPY go.sum go.sum
ENV GOTOOLCHAIN=auto
RUN go mod download

COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X github.com/vulkoingim/zrok-operator/internal/build.Version=${VERSION} -X github.com/vulkoingim/zrok-operator/internal/build.GitRevision=${GIT_REVISION} -X github.com/vulkoingim/zrok-operator/internal/build.Date=${DATE}" \
    -o manager ./cmd

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
