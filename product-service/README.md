# product-service

ระบบจัดการสินค้า — ผู้รับผิดชอบ: เอกพล แรกเรียง (67114540666)

ดู [CONTRACT.md §8.2](../CONTRACT.md#82-product-service-เจ้าของ-เอกพล) สำหรับ endpoint/RBAC
ที่ทีมตกลงกัน และ [CONTRACT.md §5.3](../CONTRACT.md#53-product-service--rental-service-ตรวจ-token-อย่างไร)
สำหรับวิธีตรวจ JWT (decode-only — Kong เป็นคนตรวจ signature ให้)

## Stack

Go + [Gin](https://github.com/gin-gonic/gin) + [GORM](https://gorm.io) (postgres driver) +
[golang-migrate](https://github.com/golang-migrate/migrate) (raw SQL migrations) — โครงสร้างเดียวกับ
`user-service` (ดูโค้ดที่นั่นเป็นตัวอย่างเพิ่มเติมได้ถ้าติดขัด)

```
cmd/api/main.go              จุดเริ่มโปรแกรม (รัน migration แล้วค่อยเปิด server)
internal/config/             อ่าน env vars
internal/db/                 เชื่อมต่อ + รัน migration
internal/model/               struct ตาราง (Category, Product)
internal/dto/                 request body + validation tags
internal/repository/          query ตรง ๆ กับ DB
internal/service/              business rule (เช่น ห้ามลบสินค้าที่กำลังเช่าอยู่)
internal/middleware/           decode JWT (ไม่ verify signature), RBAC, internal key, CORS
internal/handler/              HTTP handler
internal/router/               ผูก route ทั้งหมด
migrations/                    ไฟล์ SQL migration (golang-migrate)
```

## รันแยกเครื่อง (dev)

ไม่ต้องรอ user-service/rental-service เสร็จก่อน — รันแค่ตัวเองกับ Postgres ของตัวเองได้เลย
(CONTRACT.md §9.4)

```bash
# จากโฟลเดอร์ product-service/
cp ../.env.example .env   # หรือ export ตัวแปรด้านล่างเอง

docker run -d --name product-db -e POSTGRES_USER=product_service \
  -e POSTGRES_PASSWORD=secret -e POSTGRES_DB=product_db -p 5432:5432 postgres:16-alpine

go run ./cmd/api
```

Env vars ที่ต้องมี (ค่า default อยู่ใน `internal/config/config.go`):

| ตัวแปร | default | หมายเหตุ |
|---|---|---|
| `APP_PORT` | `8082` | |
| `DB_HOST` / `DB_PORT` | `localhost` / `5432` | |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | `product_service` / `secret` / `product_db` | |
| `INTERNAL_API_KEY` | **ต้องตั้ง** ไม่มี default | ใช้ตรวจ caller ภายใน (rental-service) ที่เรียก `PATCH /products/{id}/status` ตรง ๆ ข้าม Kong |
| `CORS_ORIGIN` | `http://localhost:3000` | |

ไม่ต้องตั้ง `JWT_SECRET` — service นี้ไม่ verify signature เอง (ดู CONTRACT.md §5.3)

### ทดสอบ endpoint ที่ต้อง login โดยไม่มี user-service

Product-service แค่ **decode** payload ของ token (ไม่ได้ verify signature เลย — Kong เป็นคนตรวจให้)
ดังนั้น token ที่ structure ถูกต้องและมี claim ที่จำเป็น ก็ใช้ทดสอบเองได้เลย เซ็นด้วย secret อะไรก็ได้:

```go
// ตัวอย่างสั้น ๆ — ดู internal/middleware/claims_test.go สำหรับโค้ดเต็ม
claims := middleware.Claims{
    RegisteredClaims: jwt.RegisteredClaims{Subject: "...", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
    Role: "admin",
}
token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
signed, _ := token.SignedString([]byte("anything"))
```

หรือขอ token จริงจากสุรเชษฐ์ (login ผ่าน user-service จริง) ก็ได้เช่นกัน

## รันเทส

```bash
go test ./...
```

ทดสอบทั้งหมดรันกับ SQLite ในหน่วยความจำ ([glebarez/sqlite](https://github.com/glebarez/sqlite), pure
Go ไม่ต้องมี cgo) ไม่ต้องมี Postgres รันอยู่เลย

## หมายเหตุการออกแบบที่ไม่ได้อยู่ใน CONTRACT.md ตรง ๆ

- **สถานะสินค้ามี 3 ค่า** ไม่ใช่ 2: `available` / `rented` / `maintenance` — ค่าที่สามเป็นการเพิ่มของ
  service นี้เอง (ปิดซ่อมชั่วคราว) ไม่ใช่ส่วนหนึ่งของ flow เช่า/คืนที่ rental-service สั่งเปลี่ยน
- **ลบสินค้าที่สถานะ `rented` ไม่ได้** (ตอบ `409 CONFLICT`) — กันไม่ให้ rental-service เหลือ
  `product_id` ที่ชี้ไปยังสินค้าที่หายไปแล้ว (ไม่มี FK ข้าม DB ให้พึ่งได้ตาม CONTRACT.md §3)
- **`PATCH /products/{id}/status`** รับได้ทั้งสองทาง: admin/staff ผ่าน Bearer token (ผ่าน Kong) **หรือ**
  rental-service เรียกตรงด้วย `X-Internal-Key` (ข้าม Kong) — ดู `middleware.RequireInternalKeyOrRole`
