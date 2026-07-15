# Next.js shadcn/ui 前端替换设计

日期：2026-07-15
状态：已确认

## 目标

以已初始化的 Next.js + shadcn/ui 项目替换现有 Vue/Vite 前端。保留全部业务流程、浏览器可见路由语义、后端 `/api/*` 契约、Cookie 会话和 SSE 快照推送；重新设计全部表现层，并把桌面端与手机端列为同等优先级。

## 不包含的范围

- 不修改 Go 后端、数据库、YYB sidecar 或任一 `/api/*` 接口的路径、方法、请求体和响应结构。
- 不增加 Next API Route、服务端转发层、数据库访问或 Node.js 常驻服务。
- 不新增统计、导出、告警、审计或其他业务功能。
- 不改变管理员权限模型、注册规则、邀请码规则或远端刷新节奏。

## 技术与发布架构

### 应用架构

- 最终前端源码目录为 `frontend/`；当前 `next-app/` 的内容在切换时成为该目录的唯一前端实现。
- 使用 Next App Router 和客户端 React 组件处理认证后的交互、浏览器 fetch、SSE 与本地 UI 状态。
- Next 配置使用 `output: "export"`，应用构建为静态 `out/`，不需要运行 Next 服务。
- 继续由现有 Nginx 服务 `frontend/dist/`；浏览器以同源方式访问 Go 后端 `/api/*`。
- 发布构建使用 pnpm。每次构建完成后执行 `rsync -a --delete out/ dist/`，确保旧 Vite hash 资源、旧 HTML 和旧静态文件不会被 Nginx 继续服务。
- `dist/`、`out/` 和 `.next/` 为构建产物，不进入 Git。
- 旧 Vue 源码不再作为发布路径；其版本可由 Git 历史恢复，必要时可以明确移动到不参与部署的 `frontend-legacy/`。

### 路由与认证

新应用提供 `/login`、`/register`、`/dashboard` 和 `/admin`。根路径根据 `/api/auth/me` 的当前角色跳转到登录页、用户看板或管理员页面。历史 `#/login`、`#/register`、`#/dashboard` 和 `#/admin` 链接在根页面加载时映射到对应的新路径。

认证边界读取 `/api/auth/me`，并保持以下行为：

- 未登录的受保护页面跳转 `/login`。
- 已登录用户访问登录或注册页时，按角色跳转到 `/dashboard` 或 `/admin`。
- 管理员和普通用户访问错误的受保护页面时跳转各自首页。
- API 401 清理客户端认证状态并返回登录页。

## 视觉与信息架构

### 共同设计系统

- 视觉方向为“开阔顶栏”和“中性专业”：白色背景、中性灰表面、深色主操作；绿色、琥珀色、红色仅表达设备或系统语义状态。
- 桌面端顶栏包含 Charge 标识、当前区域导航、页面级操作和用户菜单；手机端将导航和次级操作收进 Sheet 抽屉或全屏弹层。
- 复用 shadcn/ui 的 Button、Card、Dialog、Sheet、Tabs、Table、Alert、Skeleton、Badge、DropdownMenu、Input、Select、Field 与 sonner，不保留旧 Vue UI 基元。
- 所有触控目标最小 44×44px；键盘焦点顺序与可见顺序一致；动效遵循 `prefers-reduced-motion`。

### 认证页面

- `/login` 与 `/register` 共享认证页面，使用明确的登录/注册切换和居中表单卡片。
- 保留用户名、密码可见性开关、动态 Turnstile、注册图片验证码、邀请码可选/必填规则和注册关闭提示。
- 认证配置继续读取 `GET /api/auth/config`；注册验证码继续读取 `GET /api/auth/register-captcha`，且在注册失败后刷新。
- 成功后按原角色路由；错误消息继续以服务器返回的 `error` 为准。

### 用户工作台

- 顶栏区域为“概览、设备、账号”。
- 看板首屏展示充电桩、端口、使用中和异常端口指标；刷新状态与远端请求节流/退避信息保持可见。
- 保留搜索和状态筛选，筛选结果精确到单个充电口。
- 保留充电桩新增、编辑、删除，主动刷新、Cookie 更新、YYB 扫码绑定、使用指南确认、会话安全和退出登录。
- 仅 `/dashboard` 建立 `EventSource("/api/stream", { withCredentials: true })`；接收 `snapshot` 事件后原样替换快照，离开页面关闭连接。

### 管理员控制台

- `/admin?tab=overview|users|settings` 保留运营总览、用户管理与系统设置三页签；缺失或无效页签回退到 `overview`。
- 运营总览保留系统健康、五项运营指标、24 小时/7 天/30 天趋势、待处理异常和账户健康概览。
- 用户管理保留搜索、账户/凭据/风险筛选、分页和预取、用户详情 Sheet、创建用户，以及启停账户、设备额度、刷新权限、密码重置和删除确认。
- 系统设置保留显式保存的注册策略与邀请码生成、复制、删除；自动刷新不得覆盖未保存的设置草稿。
- 页面可见时每 60 秒刷新运营数据，进入后台停止，恢复可见时立即刷新；各 API 区域独立加载和失败，局部失败不隐藏其他区域。

## 数据边界

请求层集中封装浏览器 fetch，但不得改变下列接口的路径、HTTP 方法、JSON 请求体、`credentials: "include"`、缓存选项或非成功响应处理：

- 认证：`/api/auth/config`、`/api/auth/register-captcha`、`/api/auth/me`、`/api/auth/login`、`/api/auth/register`、`/api/auth/logout`、`/api/auth/password`、`/api/auth/sessions`、`/api/auth/sessions/others`。
- 用户看板：`/api/piles`、`/api/piles/:id`、`/api/refresh`、`/api/session/cookie`、`/api/user/usage-guide/ack`、`/api/stream`。
- YYB：`/api/session/yyb-binding`、`/api/session/yyb-qr`、`/api/session/yyb-qr/:sessionId/poll`、`/api/session/yyb-qr/:sessionId/confirm`。
- 管理员：`/api/admin/stats`、`/api/admin/health`、`/api/admin/settings`、`/api/admin/invites`、`/api/admin/users` 及其带 ID 的 PATCH/DELETE 路径。

客户端状态分为 `auth`、`dashboard`、`admin` 与 `yyb` 四个独立领域。每个领域只拥有其 API 数据、加载状态、错误和显式操作；页面组件只负责组合这些领域和 shadcn/ui 组件。

## 验收与测试

- 使用 React Testing Library 和 Vitest 将旧行为测试迁移到新实现，先证明测试失败，再实现最小功能。
- 覆盖路由权限、全部关键 fetch 契约、登录/注册、YYB 二维码生成/轮询/确认、充电桩新增编辑删除和刷新、SSE 快照、管理员页签/筛选/分页/详情/设置草稿。
- 使用 mock API 走通登录→用户看板→主动刷新→管理员控制台的核心路径。
- 在桌面视口和 375px 宽度检查认证、用户与管理员核心流程；不得出现横向滚动、不可点击的控制项或被遮挡的主要内容。
- 通过类型检查、单元测试、静态导出构建和 `out/` 到 `dist/` 的清理同步后，才允许切换发布目录。

## 成功标准

1. Nginx 服务的 `frontend/dist/` 只包含当前 Next 静态导出产物，不含旧 Vite 资源。
2. 现有后端 API 无需改动，所有浏览器请求继续是同源 `/api/*` 且携带 Cookie。
3. 普通用户和管理员能完成旧前端的全部业务操作。
4. 桌面端与手机端都使用统一的开阔顶栏、中性专业 shadcn/ui 设计语言。
5. 发布前的功能、类型、构建和响应式检查均有可重复的验证命令和结果。
