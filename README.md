# Charge Dashboard (Go + TypeScript)

基于 `20260601_202646` 抓包里记录的接口信息构建的充电桩可视化项目。

- 后端：Go（远端接口请求、内存状态、REST、SSE）
- 前端：Vue 3 + TypeScript + Vite + Pinia + Naive UI
- 功能：展示每桩充电口状态、动态新增/删除远端桩、按钮主动刷新口状态

## 目录结构

```text
backend/
  cmd/server/main.go
  internal/
    api/http.go
    charger/client.go
    model/model.go
    parser/parser.go
    store/store.go
frontend/
  src/
    App.vue
    components/PileCard.vue
    stores/dashboard.ts
    types/dashboard.ts
20260601_202646/
  0/basic
  0/request_body
  0/request_headers
```

## 后端启动

```bash
cd backend
go build -o server ./cmd/server
./server -listen :8080 -capture ../20260601_202646
```

默认会把本地状态保存到项目根目录 `charge_state.json`。这个文件已加入 `.gitignore`，用于在重启后恢复已添加的桩、上次快照、刷新时间和当前 Cookie，避免开发调试时频繁请求远端接口。也可以自定义位置：

```bash
./server -listen :8080 -capture ../20260601_202646 -state ../charge_state.json
```

`20260601_202646` 是本地抓包目录，包含 Cookie、openid、手机号等敏感信息，默认不会提交到 Git。仓库里只保留了脱敏模板：

```text
examples/capture-template/
```

本地运行时可以复制模板为自己的抓包目录，并把 `YOUR_DEVICE_LONG_ID`、`YOUR_WX_OPENID`、`YOUR_INFO_TOKEN`、`YOUR_VERIFYCODE`、`YOUR_SID` 换成你本机抓到的真实值。

启动后可访问：

- `GET /healthz`
- `GET /api/piles` 获取完整快照
- `POST /api/piles` 新增桩（使用抓包请求模板替换设备长 ID 后请求远端接口）
- `DELETE /api/piles/:id` 删除桩（同时从后续刷新列表移除）
- `POST /api/refresh` 主动刷新（30 秒内重复刷新会返回缓存，避免频繁请求远端）
- `POST /api/session/cookie` 更新 Cookie，并立即请求远端接口验证

## 前端启动

```bash
cd frontend
npm install
npm run dev
```

默认地址：`http://localhost:5173`（已代理 `/api` 到 `http://localhost:8080`）。

## 抓包使用说明

抓包目录只用于提供远端请求模板，不作为页面数据源：

- `basic` -> 远端接口地址，例如 `https://ele.mocele.com/action/i/api/devicewithnumbers`
- `request_body` -> 请求体，例如 `id=2601201412385560001`
- `request_headers` -> 必要请求头，例如 `Cookie`、`Content-Type`、`Referer`

后端刷新时会重新请求远端接口，并根据远端返回 JSON 映射：

- `id` -> 桩 ID
- `name` -> 桩名称
- `number` -> 桩号
- `opennum` -> 充电口数量（默认10）
- `status` 包含 `在线` -> 桩在线
- `used` -> 使用中的口号数组（标记为 `in_use`）

你后续把另外两个桩的抓包目录放到 `20260601_202646` 下（例如 `1/basic`, `1/request_body`, `1/request_headers`），后端刷新时会分别请求这些接口并合并成多桩展示。

也可以在页面直接新增设备长 ID，后端会用第一个抓包请求作为模板，将请求体里的 `id` 替换成新增设备长 ID 后请求远端接口。这里需要填接口使用的长 ID，例如 `2601201412385560001`，不是 `61034278` 这种短桩号。

## Cookie 过期处理

如果远端接口返回未登录、授权过期或 401/403，页面会提示 Cookie 可能已过期。此时从浏览器重新复制 `Cookie` 请求头，在页面点击“更新 Cookie”粘贴并保存即可，后端会立刻验证新 Cookie。
