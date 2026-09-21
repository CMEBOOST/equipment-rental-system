#!/usr/bin/env bash
# Exercises the Kong-fronted user-service: public routes stay open, protected
# routes require a Kong-validated JWT. Run with the compose stack already up
# (docker compose up -d --build) and Kong reachable at localhost:8000.
set -uo pipefail

BASE="http://localhost:8000/api/v1"
PASS=0
FAIL=0

check() {
  local desc="$1" expected="$2" actual="$3"
  if [ "$actual" = "$expected" ]; then
    echo "PASS: $desc (got $actual)"
    PASS=$((PASS + 1))
  else
    echo "FAIL: $desc (expected $expected, got $actual)"
    FAIL=$((FAIL + 1))
  fi
}

# 1. Public route reachable with no token at all.
code=$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8000/health)
check "GET /health (public, no token)" "200" "$code"

# 2. Register + login through Kong to obtain a real, Kong-issued-and-verifiable token.
STAMP="$(date +%s%N 2>/dev/null || date +%s)"
EMAIL="kong-smoke-${STAMP}@example.com"
USERNAME="kongsmoke${STAMP}"
PASSWORD="Str0ngPassw0rd!"

curl -s -X POST "$BASE/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" > /dev/null

LOGIN_RESP=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
TOKEN=$(echo "$LOGIN_RESP" | grep -o '"access_token":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
  echo "FAIL: login through Kong did not return an access_token (resp: $LOGIN_RESP)"
  FAIL=$((FAIL + 1))
else
  echo "PASS: register+login through Kong returned an access_token"
  PASS=$((PASS + 1))
fi

# 3. Protected route, no token -> Kong itself rejects (401), never reaches user-service.
code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/me")
check "GET /me (protected, no token)" "401" "$code"

# 4. Protected route, garbage/invalid-signature token -> Kong rejects (401).
code=$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer not-a-real-token' "$BASE/me")
check "GET /me (protected, invalid token)" "401" "$code"

# 5. Protected route, the real token from step 2 -> Kong forwards, user-service answers 200.
if [ -n "$TOKEN" ]; then
  code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $TOKEN" "$BASE/me")
  check "GET /me (protected, valid token)" "200" "$code"
fi

echo "---"
echo "$PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
