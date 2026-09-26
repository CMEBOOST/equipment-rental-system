#!/usr/bin/env bash
# End-to-end exercise of all three services together through Kong: the real
# rent -> approve -> return flow, now that product-service actually exists.
#
# Every earlier smoke test (rental-smoke-test.sh) could only prove that
# rental-service failed *gracefully* when product-service was unreachable --
# this is the first script that proves the real happy path, including the
# cross-service effect of a rental approval actually flipping a product's
# status. It also directly exercises the atomicity fix (product_repo.go's
# compare-and-swap) via a raw PATCH /products/{id}/status call (check 9b) --
# a second POST /rentals on the same product (check 9) gets rejected first
# by rental-service's own pre-check, so that call alone would still pass
# even if the compare-and-swap were reverted; 9b is what actually proves it.
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

extract() {
  # extract <key> <json> — first "<key>":"<value>" match, quoted-string values only.
  echo "$2" | grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4
}

# 1. Admin login through Kong, using the seeded admin account.
ADMIN_LOGIN_RESP=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"admin@equipment-rental.local","password":"Admin123!"}')
ADMIN_TOKEN=$(extract access_token "$ADMIN_LOGIN_RESP")
if [ -z "$ADMIN_TOKEN" ]; then
  echo "FAIL: admin login through Kong did not return an access_token (resp: $ADMIN_LOGIN_RESP)"
  FAIL=$((FAIL + 1))
  echo "---"
  echo "$PASS passed, $FAIL failed"
  exit 1
fi
echo "PASS: admin login through Kong returned an access_token"
PASS=$((PASS + 1))

# 2. GET /categories as admin through Kong — capture a seeded category id.
CATEGORIES_RESP=$(curl -s -w '\n%{http_code}' -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE/categories")
CATEGORIES_CODE=$(echo "$CATEGORIES_RESP" | tail -1)
CATEGORIES_BODY=$(echo "$CATEGORIES_RESP" | sed '$d')
check "GET /categories (admin)" "200" "$CATEGORIES_CODE"
CATEGORY_ID=$(extract id "$CATEGORIES_BODY")
if [ -z "$CATEGORY_ID" ]; then
  echo "FAIL: could not extract a seeded category id (body: $CATEGORIES_BODY)"
  FAIL=$((FAIL + 1))
  echo "---"
  echo "$PASS passed, $FAIL failed"
  exit 1
fi

# 3. POST /products as admin through Kong — a fresh product, must start "available".
STAMP="$(date +%s%N 2>/dev/null || date +%s)"
CREATE_RESP=$(curl -s -w '\n%{http_code}' -X POST "$BASE/products" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"category_id\":\"$CATEGORY_ID\",\"name\":\"e2e-smoke-${STAMP}\",\"price_per_day\":100}")
CREATE_CODE=$(echo "$CREATE_RESP" | tail -1)
CREATE_BODY=$(echo "$CREATE_RESP" | sed '$d')
check "POST /products (admin)" "201" "$CREATE_CODE"
PRODUCT_ID=$(extract id "$CREATE_BODY")
PRODUCT_STATUS=$(extract status "$CREATE_BODY")
check "new product starts available" "available" "$PRODUCT_STATUS"
if [ -z "$PRODUCT_ID" ]; then
  echo "FAIL: could not extract the new product's id (body: $CREATE_BODY)"
  FAIL=$((FAIL + 1))
  echo "---"
  echo "$PASS passed, $FAIL failed"
  exit 1
fi

# 4. Register + login a fresh customer through Kong.
EMAIL="e2e-smoke-${STAMP}@example.com"
USERNAME="e2esmoke${STAMP}"
PASSWORD="Str0ngPassw0rd!"
curl -s -X POST "$BASE/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" > /dev/null
CUSTOMER_LOGIN_RESP=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
CUSTOMER_TOKEN=$(extract access_token "$CUSTOMER_LOGIN_RESP")
if [ -z "$CUSTOMER_TOKEN" ]; then
  echo "FAIL: customer register+login through Kong did not return an access_token (resp: $CUSTOMER_LOGIN_RESP)"
  FAIL=$((FAIL + 1))
  echo "---"
  echo "$PASS passed, $FAIL failed"
  exit 1
fi
echo "PASS: customer register+login through Kong returned an access_token"
PASS=$((PASS + 1))

# 5. Customer requests a rental for the new product — must land "pending".
REQUEST_RESP=$(curl -s -w '\n%{http_code}' -X POST "$BASE/rentals/request" \
  -H "Authorization: Bearer $CUSTOMER_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"product_id\":\"$PRODUCT_ID\",\"start_date\":\"2027-01-01\",\"due_date\":\"2027-01-07\"}")
REQUEST_CODE=$(echo "$REQUEST_RESP" | tail -1)
REQUEST_BODY=$(echo "$REQUEST_RESP" | sed '$d')
check "POST /rentals/request (customer)" "201" "$REQUEST_CODE"
RENTAL_ID=$(extract id "$REQUEST_BODY")
RENTAL_STATUS=$(extract status "$REQUEST_BODY")
check "new rental starts pending" "pending" "$RENTAL_STATUS"
if [ -z "$RENTAL_ID" ]; then
  echo "FAIL: could not extract the new rental's id (body: $REQUEST_BODY)"
  FAIL=$((FAIL + 1))
  echo "---"
  echo "$PASS passed, $FAIL failed"
  exit 1
fi

# 6. A pending *request* must not reserve the product yet -- only Create/Approve do.
STILL_AVAILABLE=$(extract status "$(curl -s -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE/products/$PRODUCT_ID")")
check "product still available while rental is only pending" "available" "$STILL_AVAILABLE"

# 7. Approve the rental -- pending -> active.
APPROVE_RESP=$(curl -s -w '\n%{http_code}' -X PATCH "$BASE/rentals/$RENTAL_ID/approve" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
APPROVE_CODE=$(echo "$APPROVE_RESP" | tail -1)
APPROVE_BODY=$(echo "$APPROVE_RESP" | sed '$d')
check "PATCH /rentals/{id}/approve (admin)" "200" "$APPROVE_CODE"
check "rental is active after approval" "active" "$(extract status "$APPROVE_BODY")"

# 8. The real cross-service effect: approving the rental must have flipped
# the product to "rented" through the now-atomic PATCH /products/{id}/status.
RENTED_STATUS=$(extract status "$(curl -s -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE/products/$PRODUCT_ID")")
check "product is rented after approval" "rented" "$RENTED_STATUS"

# 9. A second attempt to rent the SAME product must now get a genuine 409 --
# this is exactly the race the atomicity fix (product_repo.go's
# compare-and-swap) closes.
SECOND_RENTAL_RESP=$(curl -s -w '\n%{http_code}' -X POST "$BASE/rentals" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$(extract user_id "$APPROVE_BODY")\",\"product_id\":\"$PRODUCT_ID\",\"start_date\":\"2027-02-01\",\"due_date\":\"2027-02-07\"}")
SECOND_RENTAL_CODE=$(echo "$SECOND_RENTAL_RESP" | tail -1)
check "second rental on an already-rented product is rejected" "409" "$SECOND_RENTAL_CODE"

# 9b. Check #9 above actually gets rejected by rental-service's own
# pre-check (product.Status != "available") before it ever calls
# PATCH /products/{id}/status -- so it does NOT, by itself, exercise the
# atomicity fix's compare-and-swap. Hit that endpoint directly, the same way
# rental-service would, to prove the CAS itself rejects the conflicting
# transition (this is the call product_repo.go's UpdateStatus guards).
DIRECT_STATUS_RESP=$(curl -s -w '\n%{http_code}' -X PATCH "$BASE/products/$PRODUCT_ID/status" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"status":"rented"}')
DIRECT_STATUS_CODE=$(echo "$DIRECT_STATUS_RESP" | tail -1)
check "PATCH /products/{id}/status to rented on an already-rented product is rejected (CAS)" "409" "$DIRECT_STATUS_CODE"

# 10. Return the rental -- active -> returned.
RETURN_RESP=$(curl -s -w '\n%{http_code}' -X PATCH "$BASE/rentals/$RENTAL_ID/return" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"return_date":"2027-01-05"}')
RETURN_CODE=$(echo "$RETURN_RESP" | tail -1)
RETURN_BODY=$(echo "$RETURN_RESP" | sed '$d')
check "PATCH /rentals/{id}/return (admin)" "200" "$RETURN_CODE"
check "rental is returned after return" "returned" "$(extract status "$RETURN_BODY")"

# 11. The product must be back to "available" after the return.
FINAL_STATUS=$(extract status "$(curl -s -H "Authorization: Bearer $ADMIN_TOKEN" "$BASE/products/$PRODUCT_ID")")
check "product is available again after return" "available" "$FINAL_STATUS"

echo "---"
echo "$PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
