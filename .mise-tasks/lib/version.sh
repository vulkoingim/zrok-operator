# shellcheck shell=bash
# Source this. Sets VERSION, GIT_REVISION, DATE, GOLDFLAGS.
# VERSION/DATE via -ldflags. GIT_REVISION is for Docker only (no .git in the
# image); local/GoReleaser binaries read vcs.revision from runtime/debug.

mod="github.com/vulkoingim/zrok-operator"

if [[ -z "${VERSION:-}" ]]; then
	VERSION="$(git describe --tags --always --dirty 2>/dev/null || true)"
fi
VERSION="${VERSION:-dev}"

if [[ -z "${GIT_REVISION:-}" ]]; then
	GIT_REVISION="$(git rev-parse --short HEAD 2>/dev/null || true)"
fi
GIT_REVISION="${GIT_REVISION:-none}"

if [[ -z "${DATE:-}" ]]; then
	DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
fi

GOLDFLAGS="-s -w"
GOLDFLAGS+=" -X ${mod}/internal/build.Version=${VERSION}"
GOLDFLAGS+=" -X ${mod}/internal/build.Date=${DATE}"
