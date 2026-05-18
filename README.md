# FULLTANK Garage API

Go/Gin API สำหรับ FULLTANK Garage รองรับระบบรับประกันสินค้า, Serial Number, Film และ Promotion สำหรับ LINE LIFF apps กับ Admin Home

## Core Endpoints

- `GET /api/serial-numbers/:serial` ตรวจ Serial Number
- `POST /api/warranty/register` ลงทะเบียนรับประกันสินค้าแบบ multipart form
- `GET /api/public/films` ข้อมูลฟิล์มสำหรับ LIFF
- `GET /api/public/promotions` ข้อมูลโปรโมชันสำหรับ LIFF
- `POST /api/auth/login` เข้าสู่ระบบ Admin

## Admin Endpoints

ทุก endpoint ด้านล่างใช้ `Authorization: Bearer <token>`

- `GET/POST /api/serial-numbers`
- `GET /api/warranty/registrations`
- `GET/POST/PATCH/DELETE /api/films`
- `GET/POST/PATCH/DELETE /api/promotions`

## Production Upload Storage

รูปภาพที่อัปโหลดจาก Admin เช่น logo ฟิล์ม, รูปโปรโมชัน, รูปแกลเลอรีฟิล์ม และบัตรรับประกัน ต้องเก็บใน persistent volume
ไม่ควรเก็บใน filesystem ของ container เพราะไฟล์จะหายได้หลัง redeploy

สำหรับ Railway ให้สร้าง Volume แล้ว mount ที่ `/data` และตั้ง Service Variable:

```env
UPLOAD_DIR=/data/uploads
```

หลังตั้งค่าแล้ว รูปที่อัปโหลดใหม่จะอยู่ใต้ `/data/uploads` และยังอยู่หลัง deploy รอบถัดไป
