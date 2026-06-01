# Charge API Dashboard

一个用于查看充电桩端口占用情况的轻量级看板。项目通过抓包模板复用远端接口请求方式，在本地用 Go 后端拉取充电桩数据，并通过 Vue + TypeScript 前端展示每个充电口的使用状态。

> 本仓库不包含真实 Cookie、openid、手机号或设备会话数据。请使用 `examples/capture-template` 创建自己的本地抓包配置。

## Features

- 充电桩看板：展示桩状态、端口数量、空闲/使用中/离线统计。
- 端口网格视图：每个桩以卡片形式展示所有充电口状态。
- 主动刷新：点击按钮后请求远端接口刷新数据。
- 刷新保护：30 秒内重复刷新会返回本地缓存，减少远端接口请求频率。
- 本地状态缓存：重启服务后恢复上一次快照、已添加设备、刷新时间和 Cookie。
- Cookie 更新：Cookie 过期时可在页面中粘贴新 Cookie，并立即验证。
- 安全模板：真实抓包目录和运行状态文件默认被 `.gitignore` 排除。

## Tech Stack

- Backend: Go, `net/http`
- Frontend: Vue 3, TypeScript, Vite, Pinia, Naive UI
- State: local JSON cache (`charge_state.json`)

## Project Structure

```text
backend/
  cmd/server/              # Go server entrypoint
  internal/api/            # REST and SSE handlers
  internal/charger/        # Remote charger API client
  internal/model/          # Shared response models
  internal/parser/         # Capture-template parser
  internal/persistence/    # Local state cache
  internal/store/          # In-memory dashboard state
frontend/
  src/
    App.vue
    components/PileCard.vue
    stores/dashboard.ts
    types/dashboard.ts
examples/capture-template/ # Sanitized request template
```

## Quick Start

### 1. Prepare Capture Template

Copy the sanitized template and fill it with your own local request data:

```bash
cp -R examples/capture-template 20260601_202646
```

Then edit:

```text
20260601_202646/0/basic
20260601_202646/0/request_body
20260601_202646/0/request_headers
```

Required values include:

- `YOUR_DEVICE_LONG_ID`
- `YOUR_WX_OPENID`
- `YOUR_INFO_TOKEN`
- `YOUR_VERIFYCODE`
- `YOUR_SID`

Do not commit the real capture directory. It is ignored by Git.

### 2. Start Backend

```bash
cd backend
go build -o server ./cmd/server
./server -listen :8080 -capture ../20260601_202646 -state ../charge_state.json
```

The backend starts at:

```text
http://localhost:8080
```

### 3. Start Frontend

```bash
cd frontend
npm install
npm run dev
```

The frontend starts at:

```text
http://localhost:5173
```

## API Overview

- `GET /healthz` health check
- `GET /api/piles` get dashboard snapshot
- `POST /api/piles` add a remote device by long device ID
- `DELETE /api/piles/:id` remove a device from the dashboard
- `POST /api/refresh` refresh remote data, with local throttle protection
- `POST /api/session/cookie` update Cookie and verify it immediately
- `GET /api/stream` server-sent event snapshot stream

## Capture Template Format

Each device request template is stored in a numbered folder:

```text
20260601_202646/
  0/
    basic
    request_body
    request_headers
```

File meanings:

- `basic`: remote API URL
- `request_body`: form body, usually including `id=YOUR_DEVICE_LONG_ID`
- `request_headers`: required headers such as `Content-Type`, `Referer`, and `Cookie`

To preload multiple devices, create more folders:

```text
20260601_202646/
  0/
  1/
  2/
```

The backend will load every valid request template and merge the returned devices into one dashboard.

## Local Cache

Runtime state is saved to:

```text
charge_state.json
```

It stores:

- added device IDs
- latest dashboard snapshot
- latest remote refresh time
- current Cookie

On restart, the server restores this file first and does not request the remote API automatically. This avoids repeated remote requests while developing or restarting the service.

## Security Notes

Never commit these files or directories:

```text
20260601_202646/
charge_state.json
.env
frontend/node_modules/
frontend/dist/
backend/server
```

Before pushing to GitHub, run:

```bash
git status --short --ignored
git add --dry-run .
```

Only code, README files, and sanitized templates should appear in the upload list.

## Cookie Expiration

If the remote API returns unauthorized, expired-session, or login-required responses, the dashboard will show a Cookie warning. Copy a fresh Cookie from your browser request headers, click `更新 Cookie`, paste it into the modal, and save. The backend validates the new Cookie immediately; if validation fails, it keeps the previous Cookie.

## Disclaimer

This project is intended for personal or internal monitoring of devices you are authorized to access. Respect the remote service's usage rules and avoid high-frequency polling.
