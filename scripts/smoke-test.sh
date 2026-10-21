#!/usr/bin/env bash
# Smoke-test a built container image: it must serve, run unprivileged, carry
# no shell, keep book titles out of its logs, and shut down cleanly.
#
# CI runs this natively on amd64 and on arm64. `make smoke` runs it locally.
set -euo pipefail

IMAGE="${1:-bookplate:smoke}"
NAME="bookplate-smoke-$$"
PORT="${PORT:-18080}"

cleanup() { docker rm -fv "$NAME" >/dev/null 2>&1 || true; }
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

# /data has to be a mount. The root filesystem is read-only, and an anonymous
# volume inherits the ownership the image carries at that path, which is what
# lets the unprivileged user create the database.
#
# MSYS_NO_PATHCONV applies to this one command: Git Bash on Windows would
# otherwise rewrite "/data" into a Windows path. Setting it for the whole
# script instead would break curl's "-o /dev/null".
MSYS_NO_PATHCONV=1 docker run -d --name "$NAME" \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  -v /data \
  -p "127.0.0.1:${PORT}:8080" "$IMAGE" >/dev/null
pass "starts read-only, no capabilities, no-new-privileges"

user=$(docker inspect "$NAME" --format '{{.Config.User}}')
[ "$user" = "nonroot:nonroot" ] || fail "runs as '$user', expected nonroot:nonroot"
pass "runs as $user"

curl -fsS --retry 30 --retry-connrefused --retry-delay 1 \
  "http://127.0.0.1:${PORT}/healthz" >/dev/null || fail "/healthz did not answer"
pass "/healthz answers"

# /readyz queries the database, so a 200 here proves the schema was created on
# a fresh volume by an unprivileged user.
curl -fsS "http://127.0.0.1:${PORT}/readyz" >/dev/null || fail "/readyz did not answer"
pass "/readyz answers, so the database opened and migrated"

if ! docker logs "$NAME" 2>&1 | grep -q "schema migrated"; then
  fail "the schema was not migrated on a fresh volume"
fi
pass "schema migrated on a fresh volume"

code=$(curl -s -w '%{http_code}' -o "$(mktemp)" "http://127.0.0.1:${PORT}/no-such-route")
[ "$code" = "404" ] || fail "an unknown route returned $code, expected 404"
pass "unknown route is 404"

# A search URL carries book titles, and what a person reads must not be
# recorded in the logs.
curl -fsS "http://127.0.0.1:${PORT}/readyz?q=a-private-title" >/dev/null
if docker logs "$NAME" 2>&1 | grep -q "a-private-title"; then
  fail "the query string reached the logs"
fi
pass "query strings stay out of the logs"

if ! curl -fsS "http://127.0.0.1:${PORT}/" | grep -q "BookPlate"; then
  fail "the interface is not served"
fi
pass "the interface is served"

headers=$(curl -fsSI "http://127.0.0.1:${PORT}/")
case "$headers" in
  *[Cc]ontent-[Ss]ecurity-[Pp]olicy*) pass "a content security policy is set" ;;
  *) fail "no content security policy on the page" ;;
esac
case "$headers" in
  *unsafe-inline*) fail "the policy allows inline script" ;;
  *) pass "the policy allows no inline script" ;;
esac

# The first run must offer setup rather than letting anyone in.
if ! curl -fsS "http://127.0.0.1:${PORT}/api/session" | grep -q '"needsSetup":true'; then
  fail "a fresh instance does not ask to be set up"
fi
pass "a fresh instance asks to be set up"

code=$(curl -s -o "$(mktemp)" -w '%{http_code}' "http://127.0.0.1:${PORT}/api/books")
[ "$code" = "401" ] || fail "the shelf answered $code without a session, expected 401"
pass "the shelf refuses an anonymous request"

# The whole journey, end to end and entirely offline: create the first
# account, add a book, read it back.
jar=$(mktemp)
json="Content-Type: application/json"
base="http://127.0.0.1:${PORT}"

curl -fsS -c "$jar" -H "$json" -o /dev/null \
  -d '{"email":"smoke@example.org","displayName":"Smoke","password":"a long enough passphrase","libraryName":"Smoke"}' \
  "$base/api/setup" || fail "first-run setup failed"
pass "first-run setup creates the administrator"

code=$(curl -s -o "$(mktemp)" -w '%{http_code}' -H "$json" \
  -d '{"email":"other@example.org","displayName":"Other","password":"another long passphrase","libraryName":"Theirs"}' \
  "$base/api/setup")
[ "$code" = "409" ] || fail "a second setup returned $code, expected 409"
pass "setup refuses to run twice"

curl -fsS -b "$jar" -H "$json" -o /dev/null \
  -d '{"title":"Fantastic Mr Fox","author":"Roald Dahl","isbn":"9780140328721"}' \
  "$base/api/books" || fail "adding a book failed"
pass "a book is added"

curl -fsS -b "$jar" "$base/api/books" | grep -q "Fantastic Mr Fox" \
  || fail "the book just added is not on the shelf"
pass "the shelf shows the book"

# Places: build two levels, file a book on the lower one, and check the count
# reaches the top. The accented name matters: this catalogue will mostly hold
# French titles.
#
# The bodies go through files. On Git Bash a non-ASCII argument is mangled on
# its way to the native curl.exe, which is a property of that shell and not of
# the server, and a test must not fail for it.
house=$(curl -fsS -b "$jar" -H "$json" -d '{"name":"Maison","kind":"building"}' \
  "$base/api/places" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
[ -n "$house" ] || fail "creating a place returned no identifier"

body=$(mktemp)
printf '{"parentId":"%s","name":"\303\211tag\303\250re 3","kind":"shelf"}' "$house" > "$body"
shelf=$(curl -fsS -b "$jar" -H "$json" --data-binary "@$body" \
  "$base/api/places" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')
[ -n "$shelf" ] || fail "creating a child place returned no identifier"
pass "a place tree is created"

expected=$(printf '\303\211tag\303\250re 3')
curl -fsS -b "$jar" "$base/api/places" | grep -q "$expected" \
  || fail "an accented place name did not survive the round trip"
pass "accented names survive"

printf '{"title":"A filed book","placeId":"%s"}' "$shelf" > "$body"
filed=$(curl -fsS -b "$jar" -H "$json" --data-binary "@$body" "$base/api/books") \
  || fail "filing a book into a place failed"
curl -fsS -b "$jar" "$base/api/books" | grep -q "\"placeName\":\"$expected\"" \
  || fail "the shelf view does not say where the book is"
pass "a book knows where it sits"

# The count has to reach every ancestor, not just the shelf it sits on.
curl -fsS -b "$jar" "$base/api/places" \
  | grep -q '"name":"Maison".*"depth":0,"copies":1' \
  || fail "the count did not reach the top of the tree"
pass "counts reach every ancestor"

# The detail view: one book, its own page, and what one person thinks of it.
copy=$(printf '%s' "$filed" | sed -n 's/.*"copyId":"\([^"]*\)".*/\1/p')
work=$(printf '%s' "$filed" | sed -n 's/.*"workId":"\([^"]*\)".*/\1/p')
[ -n "$copy" ] && [ -n "$work" ] || fail "adding a book returned no identifiers"

curl -fsS -b "$jar" "$base/api/books/$copy" | grep -q '"title":"A filed book"' \
  || fail "the detail view does not return the book"
pass "one book has its own page"

printf '{"summary":"R\303\251sum\303\251 \303\251crit \303\240 la main.","notes":"D\303\251dicac\303\251.","placeId":""}' > "$body"
summary=$(printf 'R\303\251sum\303\251 \303\251crit \303\240 la main.')
curl -fsS -b "$jar" -X PATCH -H "$json" --data-binary "@$body" "$base/api/books/$copy" \
  | grep -q "\"summary\":\"$summary\"" \
  || fail "editing the book did not keep the summary"
curl -fsS -b "$jar" "$base/api/books/$copy" | grep -q '"placeName":""' \
  || fail "an empty place did not un-file the copy"
pass "a copy is edited and un-filed"

code=$(curl -s -b "$jar" -o "$(mktemp)" -w '%{http_code}' -X PATCH -H "$json" \
  -d '{"placeId":"no-such-place"}' "$base/api/books/$copy")
[ "$code" = "400" ] || fail "moving a copy to an unknown place returned $code, expected 400"
pass "an unknown place is refused"

# A rating is in half stars, so nine is four and a half.
curl -fsS -b "$jar" -X PUT -H "$json" \
  -d '{"rating":9,"status":"read","visibility":"household","review":"Kept me up."}' \
  "$base/api/works/$work/rating" | grep -q '"average":9,"count":1' \
  || fail "the household average did not follow the rating"
curl -fsS -b "$jar" "$base/api/works/$work/rating" | grep -q '"rating":9' \
  || fail "the rating was not stored"
pass "a book is rated and the household average follows"

code=$(curl -s -b "$jar" -o "$(mktemp)" -w '%{http_code}' -X PUT -H "$json" \
  -d '{"rating":11}' "$base/api/works/$work/rating")
[ "$code" = "400" ] || fail "a rating of 11 returned $code, expected 400"
pass "a rating beyond the scale is refused"

code=$(curl -s -o "$(mktemp)" -w '%{http_code}' "$base/api/works/$work/rating")
[ "$code" = "401" ] || fail "an anonymous rating request returned $code, expected 401"
pass "ratings refuse an anonymous request"

# Searching: the accents are folded both ways, every word has to appear, and
# a barcode finds a book whether or not the hyphens were typed.
printf '{"title":"Le H\303\251risson","author":"Muriel Barbery","placeId":"%s"}' "$shelf" > "$body"
curl -fsS -b "$jar" -H "$json" --data-binary "@$body" -o /dev/null "$base/api/books" \
  || fail "adding a book to search for failed"
herisson=$(printf 'Le H\303\251risson')

curl -fsS -b "$jar" "$base/api/books?q=herisson" | grep -q "$herisson" \
  || fail "an accented title was not found by its unaccented spelling"
curl -fsS -b "$jar" "$base/api/books?q=barbery%20herisson" | grep -q "$herisson" \
  || fail "two words in any order did not find the book"
curl -fsS -b "$jar" "$base/api/books?q=978-0-14-032872-1" | grep -q "Fantastic Mr Fox" \
  || fail "a hyphenated barcode did not find the copy"
[ "$(curl -fsS -b "$jar" "$base/api/books?q=zzzznotathing")" = "[]" ] \
  || fail "a search that matches nothing returned something"
pass "a book is found by title, by author and by barcode"

# The place filter takes the subtree: the house holds the shelf.
curl -fsS -b "$jar" "$base/api/books?placeId=$house" | grep -q "$herisson" \
  || fail "filtering on the house missed what sits on its shelf"
if curl -fsS -b "$jar" "$base/api/books?placeId=none" | grep -q "$herisson"; then
  fail "a filed book appeared among the unfiled"
fi
curl -fsS -b "$jar" "$base/api/books?placeId=none" | grep -q "Fantastic Mr Fox" \
  || fail "the unfiled filter did not return an unfiled book"
pass "a place filter takes everything under it"

long=$(printf 'a%.0s' $(seq 201))
code=$(curl -s -b "$jar" -o "$(mktemp)" -w '%{http_code}' --get \
  --data-urlencode "q=$long" "$base/api/books")
[ "$code" = "400" ] || fail "a 201-character search returned $code, expected 400"
pass "an oversized search is refused"

# Validation happens before anything leaves the instance, so this needs no
# network and is safe to assert in CI.
code=$(curl -s -b "$jar" -o "$(mktemp)" -w '%{http_code}' "$base/api/lookup?isbn=9780140328722")
[ "$code" = "400" ] || fail "a mistyped ISBN returned $code, expected 400"
pass "a mistyped ISBN is refused without a round trip"

# Covers need the network, so only the refusals are asserted here.
code=$(curl -s -b "$jar" -o "$(mktemp)" -w '%{http_code}' "$base/api/covers/does-not-exist")
[ "$code" = "404" ] || fail "an unknown cover returned $code, expected 404"
pass "an unknown cover is 404"

code=$(curl -s -o "$(mktemp)" -w '%{http_code}' "$base/api/covers/does-not-exist")
[ "$code" = "401" ] || fail "an anonymous cover request returned $code, expected 401"
pass "covers refuse an anonymous request"

docker stop -t 20 "$NAME" >/dev/null
exit_code=$(docker inspect "$NAME" --format '{{.State.ExitCode}}')
[ "$exit_code" = "0" ] || fail "exited with $exit_code on SIGTERM, expected 0"
pass "shuts down cleanly on SIGTERM"

echo "smoke test passed"
