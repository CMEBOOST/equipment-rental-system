#!/usr/bin/env bash
# Exercises the Kong-fronted rental-service: the public health route, role
# gating on /rentals* and /me/rentals, and the graceful-failure path when
# rental-service calls out to product-service (which does not exist yet in
# this compose stack, so the call must fail closed with a 503, not crash).
#
# Kept separate from smoke-test.sh on purpose: that script is a pure
# user-service-through-Kong regression check, while this one depends on
# product-service *not* existing yet (asserting the graceful-failure path)
# and will need different assertions once product-service ships.
#
# Run with the compose stack already up (docker compose up -d --build) and
# Kong reachable at localhost:8000.
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

# 1. rental-service's public health route, reachable with no token at all.
code=$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8000/rental/health)
check "GET /rental/health (public, no token)" "200" "$code"

# 2. Admin login through Kong, using the seeded admin account.
ADMIN_LOGIN_RESP=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"admin@equipment-rental.local","password":"Admin123!"}')
ADMIN_TOKEN=$(echo "$ADMIN_LOGIN_RESP" | grep -o '"access_token":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$ADMIN_TOKEN" ]; then
  echo "FAIL: admin login through Kong did not return an access_token (resp: $ADMIN_LOGIN_RESP)"
  FAIL=$((FAIL + 1))
else
  echo "PASS: admin login through Kong returned an access_token"
  PASS=$((PASS + 1))
fi

# 3. Register + login a fresh customer through Kong to obtain a real,
# Kong-issued-and-verifiable token.
STAMP="$(date +%s%N 2>/dev/null || date +%s)"
EMAIL="rental-smoke-${STAMP}@example.com"
USERNAME="rentalsmoke${STAMP}"
PASSWORD="Str0ngPassw0rd!"

curl -s -X POST "$BASE/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" > /dev/null

CUSTOMER_LOGIN_RESP=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
CUSTOMER_TOKEN=$(echo "$CUSTOMER_LOGIN_RESP" | grep -o '"access_token":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "$CUSTOMER_TOKEN" ]; then
  echo "FAIL: customer register+login through Kong did not return an access_token (resp: $CUSTOMER_LOGIN_RESP)"
  FAIL=$((FAIL + 1))
else
  echo "PASS: customer register+login through Kong returned an access_token"
  PASS=$((PASS + 1))
fi

# 4. Customer requests a rental against a nonexistent product-service.
# rental-service's ProductClient must fail closed (ErrDependency -> 503
# INTERNAL_ERROR), not crash or hang, when the downstream call errors out.
if [ -n "$CUSTOMER_TOKEN" ]; then
  REQUEST_RESP=$(curl -s -w '\n%{http_code}' -X POST "$BASE/rentals/request" \
    -H "Authorization: Bearer $CUSTOMER_TOKEN" -H 'Content-Type: application/json' \
    -d '{"product_id":"11111111-1111-4111-8111-111111111111","start_date":"2026-01-01","due_date":"2026-01-07"}')
  REQUEST_CODE=$(echo "$REQUEST_RESP" | tail -1)
  REQUEST_BODY=$(echo "$REQUEST_RESP" | sed '$d')
  check "POST /rentals/request (customer, product-service unreachable)" "503" "$REQUEST_CODE"
  case "$REQUEST_BODY" in
    *INTERNAL_ERROR*)
      echo "PASS: POST /rentals/request failure body carries INTERNAL_ERROR code"
      PASS=$((PASS + 1))
      ;;
    *)
      echo "FAIL: POST /rentals/request failure body missing INTERNAL_ERROR code (body: $REQUEST_BODY)"
      FAIL=$((FAIL + 1))
      ;;
  esac
fi

# 5. Admin can list all rentals.
if [ -n "$ADMIN_TOKEN" ]; then
  code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE/rentals")
  check "GET /rentals (admin)" "200" "$code"
fi

# 6. Customer can list their own rentals.
if [ -n "$CUSTOMER_TOKEN" ]; then
  code=$(curl -s -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $CUSTOMER_TOKEN" "$BASE/me/rentals")
  check "GET /me/rentals (customer)" "200" "$code"
fi

# 7. Customer is forbidden from the admin/staff-only create endpoint.
if [ -n "$CUSTOMER_TOKEN" ]; then
  code=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/rentals" \
    -H "Authorization: Bearer $CUSTOMER_TOKEN" -H 'Content-Type: application/json' \
    -d '{"user_id":"11111111-1111-1111-1111-111111111111","product_id":"11111111-1111-1111-1111-111111111111","start_date":"2026-01-01","due_date":"2026-01-07"}')
  check "POST /rentals (customer, forbidden)" "403" "$code"
fi

echo "---"
echo "$PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
