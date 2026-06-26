# RingGlow Studio

App xử lý ảnh nhẫn bằng OpenAI Images Edit API với `gpt-image-2`. Backend Go nhận ảnh upload, gửi prompt cố định để giữ nguyên hoa văn/chi tiết nhẫn và chỉ làm vàng thật hơn theo ánh sáng studio. Frontend React 19/Vite hiển thị before/after.

## Cấu trúc

- `backend/`: Go HTTP API, endpoint `POST /api/enhance`
- `frontend/`: React 19 + Vite, design reference Minimax trong `frontend/DESIGN.md`

## Chạy backend

```powershell
cd backend
$env:OPENAI_API_KEY="your_api_key_here"
go run .
```

Backend mặc định chạy ở `http://localhost:8080`.

Env tuỳ chọn:

```powershell
$env:OPENAI_IMAGE_MODEL="gpt-image-2"
$env:OPENAI_BASE_URL="https://api.openai.com/v1"
$env:PORT="8080"
```

## Chạy frontend

```powershell
cd frontend
pnpm install
pnpm dev
```

Frontend mặc định chạy ở `http://localhost:5173` và proxy `/api` sang backend `http://localhost:8080`.

Nếu deploy frontend tách backend, đặt:

```powershell
$env:VITE_API_BASE_URL="https://your-backend.example.com/api"
```

## API

`POST /api/enhance` dùng `multipart/form-data`:

- `image`: file JPG/PNG/WEBP dưới 50MB
- `quality`: `high`, `medium`, `low`, hoặc `auto`
- `size`: `auto`, `1536x1024`, `1024x1024`, hoặc `1024x1536`
- `output_format`: `png`, `jpeg`, hoặc `webp`

Response trả `image` dạng data URL để frontend render trực tiếp.
