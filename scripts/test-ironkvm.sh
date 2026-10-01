#!/bin/sh
# Vet and unit-test the packages linked into the ironkvm build, with the
# ironkvm tag, in a golang Docker image. Runs on linux/amd64 so the tests
# can execute; RACE=1 adds the race detector (needs CGO, which the image
# has).
#
# The tests run as an unprivileged user: a few upstream tests check that
# writing to a read-only path fails, which root never sees.
#
# Usage: scripts/test-ironkvm.sh [extra go test args]
#
# Environment:
#   GO_IMAGE  Go toolchain image (default golang:1.25.14)
#   RACE      1 to run with -race
#   GOMODCACHE_VOLUME  docker volume for the module cache (default
#             picoclaw-gomod)

set -eu

GO_IMAGE=${GO_IMAGE:-golang:1.25.14}
RACE=${RACE:-0}
GOMODCACHE_VOLUME=${GOMODCACHE_VOLUME:-picoclaw-gomod}
TAGS=goolm,stdjson,ironkvm

# Upstream tests that assert the stock defaults which the ironkvm build
# changes on purpose (exec, web search, skills, cron and heartbeat off).
# defaults_ironkvm_test.go and friends check the ironkvm values instead.
SKIP='^('
SKIP="${SKIP}TestDefaultConfig_HeartbeatEnabled|TestConfig_Complete"
SKIP="${SKIP}|TestDefaultConfig_WebPreferNativeEnabled|TestLoadConfig_WebPreferNativeDefaultsTrueWhenUnset"
SKIP="${SKIP}|TestDefaultConfig_ExecAllowRemoteEnabled|TestLoadConfig_ExecAllowRemoteDefaultsTrueWhenUnset"
SKIP="${SKIP}|TestDefaultConfig_CronAllowCommandEnabled|TestLoadConfig_CronAllowCommandDefaultsTrueWhenUnset"
SKIP="${SKIP}|TestSkillsRegistriesConfigUnmarshalJSONPreservesDefaultRegistries"
SKIP="${SKIP}|TestSkillsRegistriesConfigUnmarshalJSONListPreservesDefaultRegistries"
SKIP="${SKIP}|TestSkillsRegistriesConfigUnmarshalYAMLRetainsDefaultsForOmittedFields"
SKIP="${SKIP}|TestLoadConfig_V2DirectLoad|TestToStandardConfig_ExecAllowRemoteDefaultsTrue"
SKIP="${SKIP}|TestSkillsInstallFromRegistryWritesOriginMetadata|TestSkillsInstallFromRegistryRejectsInvalidSkillArchive"
SKIP="${SKIP}|TestNewAgentLoop_RegistersWebSearchTool|TestNewAgentLoop_RegistersWebSearchTool_WhenExplicitProviderUnavailable"
SKIP="${SKIP}|TestProcessMessage_UseCommandLoadsRequestedSkill|TestProcessMessage_UseCommandArmsSkillForNextMessage"
SKIP="${SKIP}|TestApplyExplicitSkillCommand_ArmsSkillForNextMessage|TestApplyExplicitSkillCommand_InlineMessageMutatesOptions"
SKIP="${SKIP}|TestEvolutionBridge_ObserveTurnEndPayloadIncludesResolvedAttemptTrail"
SKIP="${SKIP}|TestEvolutionBridge_ObserveTurnEndUsesLatestSkillSnapshotAfterRetry|TestEvolutionBridge_TurnEndUsesExplicitAttemptTrail"
SKIP="${SKIP}|TestTurnProfile_SkillsOffAndCustomControlCatalogAndActiveSkills"
SKIP="${SKIP}|TestCronTool_.*Command.*|TestCronTool_ExecuteJobRunsCommand"
SKIP="${SKIP})\$"

ROOT=$(git rev-parse --show-toplevel)

RACE_ENV="-e CGO_ENABLED=0"
RACE_FLAG=""
if [ "$RACE" = "1" ]; then
	RACE_ENV="-e CGO_ENABLED=1"
	RACE_FLAG="-race"
fi

# MSYS_NO_PATHCONV keeps Git Bash on Windows from rewriting container paths.
export MSYS_NO_PATHCONV=1

# Fill the module cache as root, then test as an unprivileged user.
docker run --rm -v "$GOMODCACHE_VOLUME:/go/pkg/mod" -v "$ROOT:/src:ro" -w /src \
	"$GO_IMAGE" go mod download

# shellcheck disable=SC2086
docker run --rm --user 65534:65534 \
	-v "$GOMODCACHE_VOLUME:/go/pkg/mod" -v "$ROOT:/mnt/src:ro" -w /tmp \
	-e HOME=/tmp -e GOCACHE=/tmp/go-build -e "GOFLAGS=-mod=readonly -buildvcs=false" -e GOTOOLCHAIN=local \
	$RACE_ENV "$GO_IMAGE" sh -euc "
		# Some tests write next to their sources, so work on a private copy.
		mkdir /tmp/src
		tar -C /mnt/src --exclude=./.git --exclude=./build -cf - . | tar -C /tmp/src -xf -
		cd /tmp/src
		pkgs=\$(go list -tags '$TAGS' -deps -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./cmd/picoclaw \
			| grep '^github.com/sipeed/picoclaw')
		go vet -tags '$TAGS' \$pkgs
		go test -tags '$TAGS' $RACE_FLAG -count=1 -skip '$SKIP' $* \$pkgs
	"
