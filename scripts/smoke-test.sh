#!/usr/bin/env bash
# Smoke-test a built container image: it must serve, run unprivileged, carry
# no shell, keep book titles out of its logs, and shut down cleanly.
#
# CI runs this natively on amd64 and on arm64. `make smoke` runs it locally.
set -euo pipefail

IMAGE="${1:-bookplate:smoke}"
NAME="bookplate-smoke-$$"
PORT="${PORT:-18080}"

cleanup() { docker rm -f "$NAME" >/dev/null 2>&1 || true; }
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "  ok  $*"; }

arch=$(docker image inspect "$IMAGE" --format '{{.Architecture}}')
echo "Smoke-testing $IMAGE ($arch)"

# No shell: after a container breakout there must be no tooling to reuse.
if docker run --rm --entrypoint sh "$IMAGE" -c true >/dev/null 2>&1; then
  fail "the image ships a shell"
fi
pass "no shell in the image"

docker run -d --name "$NAME" \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  -p "127.0.0.1:${PORT}:8080" "$IMAGE" >/dev/null
pass "starts read-only, no capabilities, no-new-privileges"

user=$(docker inspect "$NAME" --format '{{.Config.User}}')
[ "$user" = "nonroot:nonroot" ] || fail "runs as '$user', expected nonroot:nonroot"
pass "runs as $user"

curl -fsS --retry 30 --retry-connrefused --retry-delay 1 \
  "http://127.0.0.1:${PORT}/healthz" >/dev/null || fail "/healthz did not answer"
pass "/healthz answers"

curl -fsS "http://127.0.0.1:${PORT}/readyz" >/dev/null || fail "/readyz did not answer"
pass "/readyz answers"

code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${PORT}/no-such-route")
[ "$code" = "404" ] || fail "an unknown route returned $code, expected 404"
pass "unknown route is 404"

# A search URL carries book titles, and what a person reads must not be
# recorded in the logs.
curl -fsS "http://127.0.0.1:${PORT}/readyz?q=a-private-title" >/dev/null
if docker logs "$NAME" 2>&1 | grep -q "a-private-title"; then
  fail "the query string reached the logs"
fi
pass "query strings stay out of the logs"

docker stop -t 20 "$NAME" >/dev/null
exit_code=$(docker inspect "$NAME" --format '{{.State.ExitCode}}')
[ "$exit_code" = "0" ] || fail "exited with $exit_code on SIGTERM, expected 0"
pass "shuts down cleanly on SIGTERM"

echo "smoke test passed"
