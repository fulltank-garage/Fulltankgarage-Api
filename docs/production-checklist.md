# FULLTANK Garage Production Checklist

ใช้รายการนี้ก่อน redeploy ระบบจริง เพื่อกันปัญหาไฟล์หาย, LIFF/Rich menu ไม่เปลี่ยน, และ frontend เรียก API ผิด environment

## Railway API

- ตั้ง Railway Volume ให้ API service และ mount path เป็น `/data`
- ตั้ง `UPLOAD_DIR=/data/uploads` เพื่อให้รูปโปรโมชัน/ฟิล์มอยู่ใน persistent volume ไม่หายหลัง deploy
- ตั้งค่า `DATABASE_URL`, `REDIS_URL`, `JWT_SECRET`, `LINE_CHANNEL_ACCESS_TOKEN`, `LINE_CHANNEL_SECRET` ใน Production variables
- ตั้ง Rich menu IDs ให้ครบทั้งเมนูลงทะเบียนและเมนูข้อมูลบัตรรับประกัน
- ตรวจ CORS ให้รองรับโดเมน Vercel ของ `Fulltankgarage`, `Fulltankgarage-Admin`, `Fulltankgarage-Film`, และ `Fulltankgarage-Promotion`
- หลัง deploy ให้เรียก `/health` และตรวจ log ว่า API เห็น upload directory เป็น `/data/uploads`

## Vercel Frontend

- ตั้ง `VITE_API_BASE_URL` หรือ env ที่ frontend ใช้ให้ชี้ไป Railway API production
- ตั้งชื่อแอป/manifest เป็น FULLTANK ตัวพิมพ์ใหญ่ให้ตรงทุก repo
- ตรวจ service worker/app update flow โดยเปิดแอปค้างไว้และ deploy ใหม่ 1 รอบ
- ทดสอบหน้า home app, film, promotion, และ admin หลัง deploy ด้วยข้อมูล production จริง

## Upload Verification

1. อัปโหลดรูปฟิล์ม 1 รายการจาก Admin
2. อัปโหลดรูปโปรโมชัน 1 รายการจาก Admin
3. เปิดหน้า Film และ Promotion ฝั่งลูกค้าเพื่อตรวจรูป
4. Redeploy API อีกครั้ง
5. เปิดซ้ำเพื่อยืนยันว่ารูปยังอยู่ ไม่กลับเป็นรูปแตก

## Redis Usage

- ใช้ Redis สำหรับ cache/session/realtime state ที่ไม่จำเป็นต้องอยู่ถาวร
- ห้ามเก็บไฟล์รูปใน Redis ให้เก็บใน Railway Volume หรือ object storage เท่านั้น
- ตั้ง TTL ให้ key ที่เป็น token/session/cache เสมอ

## ก่อน Push/Deploy

- Frontend: `npm run build`
- API: `go test ./...`
- ตรวจ `git status --short` ให้เหลือเฉพาะไฟล์ที่ตั้งใจแก้
