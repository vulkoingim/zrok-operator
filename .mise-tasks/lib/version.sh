# shellcheck shell=bash
# Source this. Sets VERSION, GIT_REVISION, DATE, GOLDFLAGS.
# VERSION/DATE via -ldflags. DATE is git committer time (reproducible; Kind skip-by-ID).
# GIT_REVISION is leftover for callers that still -X it; host/GoReleaser binaries
# read vcs.revision from runtime/debug. Override DATE/VERSION/GIT_REVISION via env.

mod="github.com/vulkoingim/zrok-operator"

if [[ -z "${VERSION:-}" ]]; then
	VERSION="$(git describe --tags --always --dirty 2>/dev/null || true)"
fi
VERSION="${VERSION:-dev}"

if [[ -z "${GIT_REVISION:-}" ]]; then
	GIT_REVISION="$(git rev-parse --short HEAD 2>/dev/null || true)"
fi
GIT_REVISION="${GIT_REVISION:-none}"

# Commit time, not wall clock — same commit ⇒ same binary ⇒ Kind can skip reload.
# %cI (strict ISO-8601). Apple Git rejects --date=format-utc:... (colon in strftime).
if [[ -z "${DATE:-}" ]]; then
	DATE="$(git log -1 --format=%cI 2>/dev/null || true)"
fi
DATE="${DATE:-unknown}"

GOLDFLAGS="-s -w"
GOLDFLAGS+=" -X ${mod}/internal/build.Version=${VERSION}"
GOLDFLAGS+=" -X ${mod}/internal/build.Date=${DATE}"
