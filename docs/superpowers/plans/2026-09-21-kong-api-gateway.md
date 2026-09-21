# Kong API Gateway Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Put Kong (DB-less) in front of the microservices as the single client-facing entry point, and document the resulting JWT-verification split in `CONTRACT.md`.

**Architecture:** Kong runs as one more container on `rental-net`, proxying `:8000` to each service's `:808x` port using a static, git-committed `deploy/kong/kong.yml`. Kong's `jwt` plugin validates token signature/expiry on protected routes before the request ever reaches a service; `user-service` keeps `JWT_SECRET` (it still signs tokens), the `jwt` plugin holds the same secret under a Kong "consumer," and — once they exist — `product-service`/`rental-service` only decode claims, never verify them. No RBAC, rate-limiting, or Kong DB/Admin API this round.

**Tech Stack:** Kong 3.6 (DB-less/declarative), Docker Compose, Bash (smoke test).

**Spec:** `docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md` (this plan implements it as written; read it alongside this plan — it has the full rationale and the CONTRACT.md diff table this plan applies).

## Global Constraints

- Kong image: `kong:3.6`, DB-less mode only — `KONG_DATABASE=off` + `KONG_DECLARATIVE_CONFIG`, no Kong Postgres, no Admin API usage.
- All Kong config lives in one file, `deploy/kong/kong.yml`, committed to git (`_format_version: "3.0"`).
- JWT consumer/credential: `iss` claim value `equipment-rental-system` maps to Kong consumer `rental-system-issuer`; the `jwt_secrets` entry's `secret` must be byte-for-byte the same string as `JWT_SECRET` in `.env.example` (`dev-secret-please-change-min-32-characters`). Kong's declarative config cannot read `${JWT_SECRET}` from the environment, so this duplication is accepted (same risk class as the plaintext dev secret already committed in `.env.example`).
- No new plugins beyond `jwt` on protected routes — no rate-limiting, no logging plugin, no RBAC at Kong (RBAC stays inside each service, unchanged).
- Service-to-service calls (`rental-service → product-service`, `rental-service → user-service /auth/verify`) keep calling each other directly by Docker service name; they never go through Kong.
- `product-service` and `rental-service` have no code yet (only `README.md`) and are **not** in root `docker-compose.yml`. `kong.yml` still declares routes/services for both (per the spec, forward-declared for their owners), but the `kong` entry in `docker-compose.yml` can only `depends_on` what actually exists today (`user-service`). Whoever adds `product-service`/`rental-service` to `docker-compose.yml` later also adds them to Kong's `depends_on`.
- Ports `8081`–`8083` stay published for direct debug access; `:8000` (Kong) is the only port a client should use going forward.
- The JWT `iss` claim and its test coverage already exist (`user-service/internal/service/token_service.go:15,22,42` and `token_service_test.go:31`) — no code change needed there. Do not re-touch `token_service.go`.

---

### Task 1: Kong declarative config

**Files:**
- Create: `deploy/kong/kong.yml`

**Interfaces:**
- Produces: a declarative Kong config file that `docker-compose.yml` (Task 2) mounts read-only at `/kong/kong.yml` and points `KONG_DECLARATIVE_CONFIG` at.
- Consumes: nothing from other tasks.

- [ ] **Step 1: Write `deploy/kong/kong.yml`**

```yaml
_format_version: "3.0"

consumers:
  - username: rental-system-issuer
    jwt_secrets:
      - key: equipment-rental-system   # must match claim "iss" in the token
        algorithm: HS256
        secret: dev-secret-please-change-min-32-characters  # == JWT_SECRET in .env.example

services:
  - name: user-service
    url: http://user-service:8081
    routes:
      - name: user-public
        paths:
          - /api/v1/auth/register
          - /api/v1/auth/login
          - /api/v1/auth/refresh
          - /health
        strip_path: false
      - name: user-internal
        paths:
          - /api/v1/auth/verify
        strip_path: false
        # No jwt plugin: this endpoint is called service-to-service with
        # X-Internal-Key, not a client JWT.
      - name: user-protected
        paths:
          - /api/v1/auth/logout
          - /api/v1/me
          - /api/v1/users
          - /api/v1/roles
        strip_path: false
        plugins:
          - name: jwt

  - name: product-service
    url: http://product-service:8082
    routes:
      - name: product-public
        paths: ["/health"]
        strip_path: false
      - name: product-protected
        paths:
          - /api/v1/products
          - /api/v1/categories
        strip_path: false
        plugins:
          - name: jwt

  - name: rental-service
    url: http://rental-service:8083
    routes:
      - name: rental-public
        paths: ["/health"]
        strip_path: false
      - name: rental-protected
        paths:
          - /api/v1/rentals
          - /api/v1/me/rentals
        strip_path: false
        plugins:
          - name: jwt
```

- [ ] **Step 2: Validate the file offline (no Docker Compose stack needed yet)**

Run:
```bash
docker run --rm -v "$(pwd)/deploy/kong/kong.yml:/kong/kong.yml:ro" kong:3.6 kong config parse /kong/kong.yml
```
Expected: exits `0`, no `Error:`/`error:` lines in the output (Kong prints a parse-success message). If it fails, the message names the offending key/line — fix `kong.yml` and re-run until it passes.

- [ ] **Step 3: Commit**

```bash
git add deploy/kong/kong.yml
git commit -m "feat(gateway): add Kong declarative config for the three services"
```

---

### Task 2: Wire Kong into `docker-compose.yml`

**Files:**
- Modify: `docker-compose.yml` (repo root)

**Interfaces:**
- Consumes: `deploy/kong/kong.yml` from Task 1 (mounted read-only).
- Produces: a `kong` container reachable at `localhost:8000`, proxying to the existing `user-service` container.

- [ ] **Step 1: Add the `kong` service**

In `docker-compose.yml`, add this service block alongside the existing `user-service`/`user-db` entries (before the `volumes:`/`networks:` top-level keys):

```yaml
  kong:
    image: kong:3.6
    container_name: kong
    environment:
      KONG_DATABASE: "off"
      KONG_DECLARATIVE_CONFIG: /kong/kong.yml
      KONG_PROXY_ACCESS_LOG: /dev/stdout
      KONG_ADMIN_ACCESS_LOG: /dev/stdout
      KONG_PROXY_ERROR_LOG: /dev/stderr
      KONG_ADMIN_ERROR_LOG: /dev/stderr
    volumes:
      - ./deploy/kong/kong.yml:/kong/kong.yml:ro
    ports:
      - "8000:8000"
    depends_on:
      - user-service
    networks: [rental-net]
```

`user-service`'s own `ports: ["8081:8081"]` stays as-is (direct debug access); this only adds Kong in front of it.

- [ ] **Step 2: Validate the compose file parses**

Run:
```bash
docker compose config --quiet
```
Expected: exits `0`, prints nothing (a parse/merge error would print to stderr and exit non-zero).

- [ ] **Step 3: Bring the stack up and prove Kong actually proxies**

```bash
cp -n .env.example .env
docker compose up -d --build
docker compose ps
```
Expected: `user-service`, `user-db`, and `kong` all show as running (`user-db` healthy).

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8000/health
```
Expected: `200` (Kong routed `/health` to `user-service`, matching the `user-public` route).

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8000/api/v1/me
```
Expected: `401` (no `Authorization` header — Kong's `jwt` plugin rejects it before `user-service` ever sees the request; a stopped `user-service` would still give `401` here, which is exactly the point).

- [ ] **Step 4: Commit**

```bash
git add docker-compose.yml
git commit -m "feat(gateway): add Kong service to docker-compose, proxying to user-service"
```

---

### Task 3: End-to-end smoke test through Kong

**Files:**
- Create: `deploy/kong/smoke-test.sh`

**Interfaces:**
- Consumes: the running stack from Task 2 (`docker compose up -d`, Kong on `:8000`).
- Produces: a repeatable pass/fail script future contributors (and CI, later) can run against any Kong-fronted stack.

- [ ] **Step 1: Write `deploy/kong/smoke-test.sh`**

```bash
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
```

- [ ] **Step 2: Make it executable and run it against the live stack**

```bash
chmod +x deploy/kong/smoke-test.sh
docker compose up -d --build   # no-op if Task 2's stack is already up
./deploy/kong/smoke-test.sh
```
Expected: every `check` line prints `PASS`, final line reads `5 passed, 0 failed` (register+login counts as one of the five), script exits `0`.

- [ ] **Step 3: Commit**

```bash
git add deploy/kong/smoke-test.sh
git commit -m "test(gateway): add Kong smoke test covering public/protected routes"
```

---

### Task 4: Update `CONTRACT.md`

**Files:**
- Modify: `CONTRACT.md`

**Interfaces:**
- Consumes: nothing code-level; this documents the behavior Tasks 1–3 built.
- Produces: the contract teammates read before building `product-service`/`rental-service` against Kong.

- [ ] **Step 1: §1 ports table — add the Kong row**

Locate the table at `CONTRACT.md:39-43`:

```markdown
| Service | ชื่อใน docker-compose | พอร์ต (host:container) | Base path |
|---|---|---|---|
| User Management | `user-service` | `8081:8081` | `/api/v1` |
| Product | `product-service` | `8082:8082` | `/api/v1` |
| Rental | `rental-service` | `8083:8083` | `/api/v1` |
```

Replace with:

```markdown
| Service | ชื่อใน docker-compose | พอร์ต (host:container) | Base path |
|---|---|---|---|
| **Kong (gateway)** | `kong` | `8000:8000` | — (proxy หน้าทั้ง 3 service) |
| User Management | `user-service` | `8081:8081` | `/api/v1` |
| Product | `product-service` | `8082:8082` | `/api/v1` |
| Rental | `rental-service` | `8083:8083` | `/api/v1` |

> **Client เรียกผ่าน Kong (`:8000`) เท่านั้นสำหรับ flow ปกติ** ตั้งแต่นี้ไป พอร์ต `8081`-`8083`
> ยังเปิด map ไว้เพื่อ debug/health check ตรงเท่านั้น ไม่ใช่ทางที่ client ควรใช้อีกต่อไป
```

- [ ] **Step 2: §5.2 JWT claims — add `iss`**

Locate the claims block at `CONTRACT.md:181-191`:

```json
{
  "sub": "<user UUID>",
  "email": "somchai@example.com",
  "username": "somchai",
  "role": "customer",
  "iat": 1757148000,
  "exp": 1757148900,
  "jti": "<uuid>"
}
```

Replace with:

```json
{
  "sub": "<user UUID>",
  "email": "somchai@example.com",
  "username": "somchai",
  "role": "customer",
  "iss": "equipment-rental-system",
  "iat": 1757148000,
  "exp": 1757148900,
  "jti": "<uuid>"
}
```

Add a line right after the closing ` ``` `:

```markdown
`iss` เป็นค่าคงที่เดียวกันทุก token (ไม่ผูกกับ user) — Kong's `jwt` plugin ใช้ค่านี้จับคู่กับ
credential ที่ตั้งไว้ใน `deploy/kong/kong.yml` เพื่อเลือก secret มาตรวจลายเซ็น
```

- [ ] **Step 3: §5.3 rewrite — Kong verifies, downstream services only decode**

Locate `CONTRACT.md:193-214` (heading `### 5.3 product-service / rental-service ตรวจ token อย่างไร` through the line before `### 5.4 Refresh token`). Replace the entire section body with:

```markdown
### 5.3 product-service / rental-service ตรวจ token อย่างไร

**Kong ตรวจ signature/expiry ให้แล้วก่อน request จะมาถึง service** (ผ่าน `jwt` plugin บน route
ที่ต้อง login ใน `deploy/kong/kong.yml`) ดังนั้น `product-service` และ `rental-service`:

1. **ไม่ต้องถือ `JWT_SECRET`** และ**ไม่ต้อง verify signature เอง**
2. อ่าน header `Authorization: Bearer <token>` (Kong forward ให้โดยไม่ตัดออก)
3. Decode ส่วน payload (base64) อ่าน claims `sub` / `role` / `email` ไปใช้บังคับสิทธิ์ — ไม่ต้อง verify signature ซ้ำ
4. เรียกตรงพอร์ต 8082/8083 (ข้าม Kong) จะไม่มีใครเช็ค signature — ต้องทดสอบ reject flow ผ่าน Kong (`:8000`) เท่านั้น

**กรณีต้องมั่นใจว่าบัญชียัง active / ไม่ถูกแบน** — เรียก introspection ของ user-service เหมือนเดิม
(ไม่เปลี่ยน จาก Kong ไม่ทำ RBAC หรือเช็ค active/ban):

```
POST http://user-service:8081/api/v1/auth/verify
Header: X-Internal-Key: <INTERNAL_API_KEY>
Body:   { "token": "<access token>" }
```

ตอบ `200` พร้อม `{ active, user_id, email, username, role, expires_at }`
หรือ `401` ถ้า token ใช้ไม่ได้

> แนะนำ: ใช้ claims ที่ decode จาก Kong เป็นหลัก เรียก `/auth/verify` เฉพาะ action สำคัญ
> (เช่น ยืนยันการเช่า) เพื่อลด network call — endpoint นี้เรียกตรงข้าม container เหมือนเดิม ไม่ผ่าน Kong
```

- [ ] **Step 4: §7.1 — note that service-to-service calls bypass Kong**

Locate `CONTRACT.md:260-273` (the `### 7.1 ใครเรียกใคร` block, including the diagram and the line right after it). After the existing line:

```markdown
> ทางเลือกที่ง่ายกว่าการให้ product เรียก rental: ให้ **rental-service เป็นคนอัปเดตสถานะ** โดยเรียก `PATCH /products/{id}/status` ของ product-service ตอนเช่า/คืน — ทีมเลือกแนวทางนี้ร่วมกัน
```

Add:

```markdown
> **Kong ไม่เกี่ยวกับ diagram ข้างบนนี้เลย** — ทุกลูกศรใน 7.1 (`rental→product`, `rental→user`,
> `product→rental`) ยังเรียกตรงข้าม container name เหมือนเดิม ไม่ผ่าน Kong (`:8000`) Kong เป็นแค่
> ทางเข้าสำหรับ **client** เท่านั้น
```

- [ ] **Step 5: §9.2 docker-compose — add the `kong` service block**

Locate `CONTRACT.md:370-445` (the ` ```yaml ` block under `### 9.2`). Immediately before the closing ` ``` ` (before `volumes:` at line 436), insert:

```yaml
  kong:
    image: kong:3.6
    environment:
      KONG_DATABASE: "off"
      KONG_DECLARATIVE_CONFIG: /kong/kong.yml
      KONG_PROXY_ACCESS_LOG: /dev/stdout
      KONG_ADMIN_ACCESS_LOG: /dev/stdout
      KONG_PROXY_ERROR_LOG: /dev/stderr
      KONG_ADMIN_ERROR_LOG: /dev/stderr
    volumes:
      - ./deploy/kong/kong.yml:/kong/kong.yml:ro
    ports:
      - "8000:8000"
    depends_on:
      - user-service
      - product-service
      - rental-service
    networks: [rental-net]

```

Directly under the closing ` ``` ` of that code block, add a note (this snippet shows the eventual full 3-service `depends_on`; today's real root `docker-compose.yml` only has `user-service`, so Kong's `depends_on` there lists just `user-service` until the other two exist):

```markdown
> **สถานะปัจจุบัน (2026-09-21):** `product-service`/`rental-service` ยังไม่มีโค้ด ใน
> docker-compose.yml จริงตอนนี้ `kong.depends_on` จึงมีแค่ `user-service` — คนที่เพิ่ม
> product-service/rental-service เข้า compose ทีหลัง ต้องเพิ่มชื่อ service นั้นเข้า
> `kong.depends_on` ด้วย
```

- [ ] **Step 6: §9.3 run commands — add the Kong curl example**

Locate `CONTRACT.md:447-456`:

```markdown
### 9.3 คำสั่งรันทั้งระบบ

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps          # เช็คว่าทุกตัว healthy
curl http://localhost:8081/health
curl http://localhost:8082/health
curl http://localhost:8083/health
```
```

Replace with:

```markdown
### 9.3 คำสั่งรันทั้งระบบ

```bash
cp .env.example .env
docker compose up -d --build
docker compose ps          # เช็คว่าทุกตัว healthy
curl http://localhost:8081/health   # ตรง — debug เท่านั้น
curl http://localhost:8082/health   # ตรง — debug เท่านั้น
curl http://localhost:8083/health   # ตรง — debug เท่านั้น
curl http://localhost:8000/api/v1/products   # ผ่าน Kong — ทางที่ client ควรใช้
```
```

- [ ] **Step 7: §9.4 — add the note about testing reject-flows through Kong**

Locate `CONTRACT.md:458-463`:

```markdown
### 9.4 พัฒนาแยกเครื่อง

- แต่ละคนรันแค่ service + db ของตัวเองระหว่างพัฒนา
- user-service ไม่พึ่งใคร รันได้เลย
- product / rental ต้องการ JWT ทดสอบ → โคลน user-service มารัน หรือขอ test token จากสุรเชษฐ์
- นัด integrate รวม (ทั้ง 3 service) อย่างน้อยสัปดาห์ละ 1 ครั้ง
```

Add one bullet at the end:

```markdown
- ทดสอบว่า token ปลอม/หมดอายุถูก reject จริง ต้องยิงผ่าน Kong (`:8000`) เท่านั้น — solo dev ที่ทดสอบ
  business logic ปกติไม่ต้องผ่าน Kong ก็ได้ (ยิง mock claims ตรง port service ได้เลย)
```

- [ ] **Step 8: Checklist — add the Kong confirmation item**

Locate `CONTRACT.md:524-534`. After the existing line:

```markdown
- [ ] วิธีตรวจ JWT ของ product/rental — ตรวจเอง vs เรียก `/auth/verify` (ข้อ 5.3)
```

Add:

```markdown
- [ ] ยืนยันการใช้ Kong เป็น entry point + ย้าย JWT verify ไป gateway (ข้อ 1, 5.2, 5.3, 9.2)
```

- [ ] **Step 9: Changelog — add v2**

Locate `CONTRACT.md:516-521`:

```markdown
## Changelog

| เวอร์ชัน | วันที่ | การเปลี่ยนแปลง | โดย |
|---|---|---|---|
| v1 (ร่าง) | 2026-09-06 | ร่างฉบับแรก | สุรเชษฐ์ |
```

Replace with:

```markdown
## Changelog

| เวอร์ชัน | วันที่ | การเปลี่ยนแปลง | โดย |
|---|---|---|---|
| v1 (ร่าง) | 2026-09-06 | ร่างฉบับแรก | สุรเชษฐ์ |
| v2 | 2026-09-21 | เพิ่ม Kong API Gateway เป็น single entry point, ย้าย JWT verify ไป gateway | สุรเชษฐ์ |
```

- [ ] **Step 10: Read the whole diff back and check every row of the spec's diff table**

Run `git diff CONTRACT.md` and confirm, against `docs/superpowers/specs/2026-09-18-kong-api-gateway-design.md` §7's table, that all nine rows (พอร์ต, 5.2, 5.3, 7, 9.2, 9.3, 9.4, Checklist, Changelog) are reflected. Fix anything missing before committing.

- [ ] **Step 11: Commit**

```bash
git add CONTRACT.md
git commit -m "docs(contract): document Kong as the gateway and the JWT-verification split"
```

---

## Known Limitations (carried from the spec, not fixed by this plan)

- `deploy/kong/kong.yml` declares an identical `/health` path under all three services' routes (`user-public`, `product-public`, `rental-public`). Kong's router resolves path collisions by its own priority rules (regex specificity / config order), not by which container is actually reachable. Today this is inert — `product-service`/`rental-service` don't exist, so only the `user-service` `/health` route can ever match — but once those services are added, whoever wires them in should verify `curl :8000/health` still reaches the intended service, or split the paths (e.g. per-service health path) if it doesn't.
- `product-service` and `rental-service` routes/JWT enforcement in `kong.yml` are untestable until those services exist; Task 3's smoke test only exercises `user-service`.

## Execution Handoff

Two ways to run this plan:

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks.
2. **Inline Execution** — execute tasks in this session with `executing-plans`, checkpoint between tasks.
