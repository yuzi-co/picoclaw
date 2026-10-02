#!/bin/sh
# Build the trimmed PicoClaw binary for IronKVM (SG2002, linux/riscv64).
#
# The build runs in a pinned golang Docker image from a `git archive` of the
# given revision (HEAD by default), so the result depends only on the commit
# and the toolchain, not on the local checkout (line endings, untracked or
# modified files). Two runs on the same commit give the same sha256.
#
# Usage: scripts/build-ironkvm.sh [revision]
#
# Environment:
#   GO_IMAGE   Go toolchain image (default golang:1.25.14)
#   OUT_DIR    output directory (default build/)
#   GO_TAGS    build tags (default goolm,stdjson,ironkvm; never add
#              whatsapp_native, it links GPL-3.0 libsignal)
#   GOMODCACHE_VOLUME  docker volume for the module cache (default
#              picoclaw-gomod; set to an empty string to use none)
#   DOCKER     container runtime that takes docker's run options (default
#              docker; IronKVM's tooling passes wslc)

set -eu

REV=${1:-HEAD}
GO_IMAGE=${GO_IMAGE:-golang:1.25.14}
GO_TAGS=${GO_TAGS:-goolm,stdjson,ironkvm}
GOMODCACHE_VOLUME=${GOMODCACHE_VOLUME-picoclaw-gomod}
DOCKER=${DOCKER:-docker}

case ",$GO_TAGS," in
*,whatsapp_native,*)
	echo "refusing to build with whatsapp_native" >&2
	exit 1
	;;
esac

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"
OUT_DIR=${OUT_DIR:-$ROOT/build}
mkdir -p "$OUT_DIR"

COMMIT=$(git rev-parse --short=8 "$REV^{commit}")
VERSION=$(git describe --tags --match 'v[0-9]*' --always "$REV" 2>/dev/null || echo "$COMMIT")
# Use the commit time, not the wall clock, so the binary is reproducible.
BUILD_TIME=$(git log -1 --format=%cI "$REV")
OUT_NAME=picoclaw-linux-riscv64-ironkvm

CONFIG_PKG=github.com/sipeed/picoclaw/pkg/config
LDFLAGS="-s -w -X $CONFIG_PKG.Version=$VERSION -X $CONFIG_PKG.GitCommit=$COMMIT -X $CONFIG_PKG.BuildTime=$BUILD_TIME"

MOD_MOUNT=""
if [ -n "$GOMODCACHE_VOLUME" ]; then
	MOD_MOUNT="-v $GOMODCACHE_VOLUME:/go/pkg/mod"
fi

# MSYS_NO_PATHCONV keeps Git Bash on Windows from rewriting /out and /src.
# shellcheck disable=SC2086
git archive --format=tar "$REV" | MSYS_NO_PATHCONV=1 "$DOCKER" run --rm -i \
	$MOD_MOUNT \
	-v "$OUT_DIR:/out" \
	-e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=riscv64 \
	-e GOFLAGS=-mod=readonly -e GOTOOLCHAIN=local \
	"$GO_IMAGE" sh -euc "
		mkdir -p /src && tar -x -C /src && cd /src
		go version
		go build -tags '$GO_TAGS' -trimpath -buildvcs=false \
			-ldflags '$LDFLAGS' -o /out/$OUT_NAME ./cmd/picoclaw
		cd /out && sha256sum $OUT_NAME > $OUT_NAME.sha256
		chown $(id -u):$(id -g) $OUT_NAME $OUT_NAME.sha256
	"

echo "built $OUT_DIR/$OUT_NAME ($VERSION, tags $GO_TAGS)"
ls -l "$OUT_DIR/$OUT_NAME"
cat "$OUT_DIR/$OUT_NAME.sha256"
