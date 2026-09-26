# rental-service

ระบบจัดการการเช่า — ผู้รับผิดชอบ: วิษณุพงศ์ บัวเขียว (67114540509)

บริการนี้จัดการคำขอเช่า การสร้างรายการเช่า การอนุมัติ และการคืนสินค้า โดยใช้ PostgreSQL
เป็นฐานข้อมูลของตัวเอง และเรียก product-service/user-service ผ่าน HTTP ภายใน Docker network
ตาม [CONTRACT.md](../CONTRACT.md)

## Endpoints

Base path คือ `/api/v1` ทุก endpoint นอกจาก health ต้องมี Bearer token ที่ผ่าน Kong แล้ว

| Method | Path | สิทธิ์ |
| --- | --- | --- |
| GET | `/health` | public |
| GET | `/rentals` | admin, staff |
| GET | `/rentals/{id}` | admin, staff, เจ้าของรายการ |
| POST | `/rentals` | admin, staff |
| POST | `/rentals/request` | customer |
| PATCH | `/rentals/{id}/approve` | admin, staff |
| PATCH | `/rentals/{id}/return` | admin, staff |
| GET | `/me/rentals` | authenticated |

การคิดราคาใช้ `price_per_day × (due_date - start_date)` และบันทึกเป็นราคาของรายการเช่า
จึงไม่เปลี่ยนตามราคาสินค้าในภายหลัง วันที่ใน request ใช้ `YYYY-MM-DD` และ `due_date`
ต้องมากกว่า `start_date`

## การทำงานร่วมกับบริการอื่น

- `GET /products/{id}`: ตรวจสินค้า สถานะ และ `price_per_day`
- `PATCH /products/{id}/status`: เปลี่ยนเป็น `rented` เมื่อเริ่มเช่า และ `available` เมื่อตีกลับหรือคืน
- `POST /auth/verify`: ตรวจ access token ของผู้ที่สั่งให้เกิดการเปลี่ยนสถานะ เพื่อยืนยันว่าบัญชียังใช้งานได้

## รันด้วย Docker

จาก root ของ repository:

```bash
cp .env.example .env
docker compose up -d --build
curl http://localhost:8000/rental/health
```

`/rental/health` เป็น public route ผ่าน Kong ส่วน API ปกติเรียกผ่าน `localhost:8000`
