# Charge Console 生产部署与运维指南

本文面向使用 Linux、systemd 和反向代理的自托管部署。首次安装、升级、密钥、备份和排障都以仓库当前提供的脚本与服务模板为准。

> [!IMPORTANT]
> 升级已有实例前先备份 SQLite，并保存与数据库匹配的加密密钥。不要让旧版本程序直接打开已经由新版本升级过的数据库。

## 升级说明（1.5.5）

- 前后端必须同时更新到同一版本；首次启动会自动增量迁移 SQLite schema，不删除原有业务数据。
- **`yyb_go` 扫码 sidecar 已退役**：`charge-server` 现已忽略 `YYB_BASE_URL`，改为「个人 Cookie + 互助公共 Cookie 池 + 全站共享公共桩库」模式；旧实例升级后可停用并下线原有 `/opt/yyb_go/yyb-go`（原监听 `127.0.0.1:8000`）。
- 更新后检查 `/healthz` 返回版本 `1.5.5`，并用管理员确认公共 Cookie 池、公共桩库与公告管理正常。
- 公告与个性化状态接口不得配置 CDN 共享缓存；`/_next/static/` 资源由后端输出长期缓存头，404 响应统一返回 `Cache-Control: no-store`；页面严格 CSP 已放行 Cloudflare Web Analytics。
- 回滚前若涉及数据库 schema 变更，应停服后同时恢复旧版程序、前端静态产物与升级前数据库备份；若仅因新版本启动失败触发自动回滚，部署脚本会自动恢复上一个版本的 `frontend/dist` 与 `charge-server`。

## 推荐拓扑

```text
Internet
   │ HTTPS
Nginx / Caddy / Cloudflare
   │ http://127.0.0.1:8080
charge-server
   ├── /var/lib/charge-api/charge_state.db (SQLite WAL)
   ├── 远端充电服务（个人 Cookie / 互助公共 Cookie 池）
   └── WxPusher HTTPS API（可选）
```

基本边界：

- 只有 HTTPS 反向代理对公网开放。
- `charge-server` 仅监听 `127.0.0.1:8080`。
- SQLite、环境文件和备份不放在 Git 仓库中。

## 服务器要求

推荐 Ubuntu 或 Debian，并准备：

- Git、Make、rsync、curl
- Node.js 22 与 pnpm 11
- Go 1.25，或支持按 `go.mod` 自动下载 toolchain 的 Go 版本
- sqlite3
- Nginx 或 Caddy
- systemd

Ubuntu/Debian 示例：

```bash
sudo apt update
sudo apt install -y git make rsync curl sqlite3 nginx
curl -fsSL https://deb.nodesource.com/setup_22.x | sudo -E bash -
sudo apt install -y nodejs
sudo npm install -g pnpm@11
```

Go 请按 [go.dev/doc/install](https://go.dev/doc/install) 安装。可以用以下命令确认环境：

```bash
git --version
go version
node --version
pnpm --version
sqlite3 --version
```

## 首次安装

### 1. 创建用户和目录

```bash
sudo useradd --system --home /nonexistent --shell /usr/sbin/nologin charge
sudo install -d -o charge -g charge -m 0750 /var/lib/charge-api
sudo git clone https://github.com/ninuan/charge-api.git /opt/charge-api
```

如果已存在 `charge` 用户或仓库目录，跳过对应命令。

### 2. 构建前后端

```bash
cd /opt/charge-api
make setup
pnpm --dir frontend run build:static
cd backend
go build -o charge-server ./cmd/server
```

Go 服务从 `frontend/dist/` 提供静态页面，因此前端构建产物和后端二进制必须来自同一版本。

如需在生产环境启用 Cloudflare Web Analytics，因为静态导出在 **构建期** 内联 `NEXT_PUBLIC_*` 环境变量（运行时 `/etc/charge-api.env` 不会生效），请在服务器仓库的 `frontend/.env.production.local`（已被 `.gitignore` 忽略且不会覆盖远端）写入：

```env
NEXT_PUBLIC_CF_BEACON_TOKEN=your-cloudflare-analytics-token
```

未配置时不会加载任何外部统计脚本。

### 3. 配置环境变量

先生成密钥：

```bash
cd /opt/charge-api
./scripts/gen_secrets.sh
```

将实际值写入 `/etc/charge-api.env`（只需填入 `CHARGE_COOKIE_KEY` 及所需可选配置）：

```env
CHARGE_ADMIN_PASSWORD=replace-with-a-strong-initial-password
CHARGE_COOKIE_KEY=replace-with-base64-encoded-32-byte-key

# 可选：WxPusher 微信提醒
WXPUSHER_APP_TOKEN=AT_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
PUBLIC_BASE_URL=https://charge.example.com

# 仅当前后端跨域部署时填写；同源部署留空
# CORS_ALLOWED_ORIGINS=https://console.example.com
```

收紧权限：

```bash
sudo chown root:charge /etc/charge-api.env
sudo chmod 0600 /etc/charge-api.env
```

注意：

- `CHARGE_ADMIN_PASSWORD` 只在首次创建数据库时生效，不会重置已有管理员密码。
- 未提供初始密码时，服务会生成一次性密码并保存到 `/var/lib/charge-api/initial-admin-password.txt`；首次登录后应删除该文件。
- `CHARGE_COOKIE_KEY` 用于加密个人与公共池 Cookie、WxPusher UID 等敏感数据，必须长期保存。
- `PUBLIC_BASE_URL` 必须是 HTTPS Origin，例如 `https://charge.example.com`；不能包含路径、查询参数、凭据或结尾斜杠。
- 生产环境不要设置 `WXPUSHER_BASE_URL`，让程序使用 WxPusher 官方 HTTPS API。
- AppToken 只能放在服务端环境文件中，不能使用 `NEXT_PUBLIC_*` 或提交到 Git。

### 4. 安装 systemd 服务

仓库提供：

```text
deploy/systemd/charge.service
deploy/systemd/charge-backup.service
deploy/systemd/charge-backup.timer
```

安装 Charge 服务：

```bash
sudo cp /opt/charge-api/deploy/systemd/charge.service /etc/systemd/system/charge-api.service
sudo systemctl daemon-reload
sudo systemctl enable --now charge-api
```

模板中的 Charge 启动命令为：

```bash
/opt/charge-api/backend/charge-server \
  -listen 127.0.0.1:8080 \
  -database /var/lib/charge-api/charge_state.db \
  -state /var/lib/charge-api/charge_state.json
```

服务模板启用了 `NoNewPrivileges`、`PrivateTmp`、`ProtectSystem=strict` 和 `UMask=0077`；Charge 只允许写入 `/var/lib/charge-api`。

### 5. 配置反向代理

Nginx 最小示例：

```nginx
server {
    listen 443 ssl http2;
    server_name charge.example.com;

    # 在此配置 ssl_certificate 与 ssl_certificate_key

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /api/stream {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
        proxy_read_timeout 1h;
    }
}
```

生产环境默认只允许同源请求。只有前后端确实使用不同 Origin 时，才配置 `CORS_ALLOWED_ORIGINS`；不要使用 `*`。

### 6. 验证服务

```bash
curl -fsS http://127.0.0.1:8080/healthz
sudo systemctl status charge-api --no-pager
sudo journalctl -u charge-api -n 100 --no-pager
```

启用 WxPusher 后，再由普通用户在「账号 → 通知与提醒」完成“获取二维码 → 微信扫码 → 发送测试消息”。“WxPusher 已处理”表示供应商完成处理，不等同于某台微信客户端已经展示或已读。

## 环境变量

### Charge

| 变量                    | 默认值   | 说明                                      |
| ----------------------- | -------- | ----------------------------------------- |
| `CHARGE_ADMIN_PASSWORD` | 自动生成 | 首次初始化管理员密码。                    |
| `CHARGE_COOKIE_KEY`     | 无       | 32 字节 Base64 加密密钥；生产必需。       |
| `CORS_ALLOWED_ORIGINS`  | 同源     | 逗号分隔的允许 Origin。                   |
| `TRUSTED_PROXY_HEADER`  | 自动回环 | 反向代理传递客户端 IP 的受信任请求头（`none`、`cf-connecting-ip`、`x-forwarded-for`、`x-real-ip`）。 |
| `WXPUSHER_APP_TOKEN`    | 关闭     | WxPusher 服务端 AppToken。                |
| `PUBLIC_BASE_URL`       | 无       | 启用 WxPusher 时必需的公网 HTTPS Origin。 |
| `WXPUSHER_BASE_URL`     | 官方 API | 仅用于本地 mock 或受控私有代理。          |
| `NEXT_PUBLIC_CF_BEACON_TOKEN` | 关闭 | 前端构建期变量（写入 `frontend/.env.production.local`），仅生产构建非空时启用 Cloudflare Web Analytics。 |

### 备份服务

`charge-backup.service` 可从 `/etc/charge-backup.env` 读取覆盖项。常见变量包括备份目录、数据库路径和保留天数；可直接查看 [`scripts/backup_db.sh`](../scripts/backup_db.sh) 中的默认值。

### 已退役组件：下线旧版 `yyb_go` sidecar

如果服务器早年部署过 `/opt/yyb_go/yyb-go`（监听 `127.0.0.1:8000`），升级到 1.5.5 后可安全下线：

```bash
sudo systemctl disable --now yyb-go || true
```

同时可从 `/etc/charge-api.env` 中移除旧的 `YYB_BASE_URL` 与 `YYB_API_SECRET`（未移除时新版 `charge-server` 也会在启动日志中忽略）。

## 更新部署

### 推荐：Git 同步部署

在开发机仓库运行：

```bash
make deploy-git DEPLOY_HOST=root@your-server
```

脚本会：

1. 校验 `DEPLOY_HOST`、`SERVICE_NAME`、`HEALTH_URL` 和 `DEPLOY_PATH`（前置校验，避免误操作）。
2. 确认本地工作区没有未提交修改，并运行 `make check`。
3. 将当前分支推送到 GitHub。
4. 通过 SSH 在服务器执行 `git pull --ff-only` 与前端源码检查。
5. 安装前端依赖、构建静态页面并编译新的 Go 二进制。
6. 原子切换 `frontend/dist` 与 `charge-server`（保留 `.prev` 备份）。
7. 重启 systemd 服务并轮询 `/healthz`；若重启或健康检查失败，自动输出 `systemctl status` 与 `journalctl` 诊断日志并回滚 `frontend/dist` 与 `charge-server`。

自定义服务器路径、分支和服务名：

```bash
make deploy-git \
  DEPLOY_HOST=root@your-server \
  DEPLOY_PATH=/opt/charge-api \
  DEPLOY_BRANCH=main \
  SERVICE_NAME=charge-api
```

服务器访问 npm 官方源不稳定时：

```bash
NPM_REGISTRY=https://registry.npmmirror.com \
PNPM_FETCH_TIMEOUT=180000 \
PNPM_FETCH_RETRIES=8 \
make deploy-git DEPLOY_HOST=root@your-server
```

预演命令，不推送也不连接服务器：

```bash
SKIP_CHECK=1 make deploy-git \
  DEPLOY_HOST=root@your-server \
  DEPLOY_ARGS=--dry-run
```

### 备用：rsync 直传

GitHub 临时不可用时：

```bash
make deploy DEPLOY_HOST=root@your-server
```

该脚本会排除 `.local/`、`.env`、SQLite、Cookie 密钥、`node_modules/` 和本地构建目录等运行数据，同样具备原子切换与失败自动回滚能力。

## 备份与恢复

Charge 数据库路径为：

```text
/var/lib/charge-api/charge_state.db
```

不要在服务运行时只复制主 `.db` 文件；启用 WAL 后可能得到不完整备份。仓库提供的脚本使用 SQLite 在线备份 API。

安装每日备份 timer：

```bash
sudo apt install -y sqlite3
sudo cp /opt/charge-api/deploy/systemd/charge-backup.service /etc/systemd/system/
sudo cp /opt/charge-api/deploy/systemd/charge-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now charge-backup.timer
```

默认每天 03:30 执行，并随机延迟最多 15 分钟；备份任务以 `charge:charge` 身份运行，写入 `/var/lib/charge-api/backups/`，使用 `0600` 权限并滚动保留 14 天。

> [!TIP]
> 若服务器此前安装过旧版（root 身份运行的）`charge-backup.service`，更新 systemd 单元后请执行一次 `sudo chown -R charge:charge /var/lib/charge-api/backups`，确保备份任务与运维监控页均可正常读写历史备份目录。

手动触发：

```bash
sudo systemctl start charge-backup.service
sudo journalctl -u charge-backup.service -n 100 --no-pager
```

恢复前验证压缩备份：

```bash
gunzip -k /var/lib/charge-api/backups/charge_state-<时间戳>.db.gz
sqlite3 /var/lib/charge-api/backups/charge_state-<时间戳>.db "PRAGMA integrity_check;"
```

备份包含加密后的用户与公共池凭据，应加密异地保存并限制权限。恢复清单必须同时包含原始的 `CHARGE_COOKIE_KEY`。

## 安全加固

### 检查监听地址

```bash
ss -lntp | grep -E ':8080|:8000'
```

预期只看到 `127.0.0.1:8080`，且不应有 `0.0.0.0:8080` 或已退役的 `8000/tcp` 对外暴露；如需收紧防火墙：

```bash
sudo ufw deny 8000/tcp
sudo ufw status
```

### 检查文件权限

```bash
stat -c '%a %n' \
  /etc/charge-api.env \
  /var/lib/charge-api/charge_state.db
```

环境文件和数据库推荐为 `0600`。

### 运行安全检查

```bash
cd /opt/charge-api
./scripts/security_check.sh --dry-run
```

默认检查 `charge-api` 的 `/healthz`、回环端口 `8080`、主库与 WAL 文件及 `journalctl -u charge-api` 最近日志中是否出现敏感明文。

## 升级与回滚

升级前：

1. 创建在线备份并执行 `PRAGMA integrity_check;`。
2. 保存当前二进制、Git 提交号和数据库 Schema 对应的发布说明。
3. 确认 `CHARGE_COOKIE_KEY` 已有离线安全副本。
4. 阅读目标版本的 [`RELEASE_NOTES.md`](../RELEASE_NOTES.md) 和 [`CHANGELOG.md`](../CHANGELOG.md)。

部署后：

1. `/healthz` 返回预期版本。
2. 普通用户可以登录、加载看板、从公共桩库添加设备并主动刷新。
3. 管理员运维页、公共 Cookie 池及运营趋势无异常报错。
4. 临时空闲提醒与已启用的 WxPusher 分别完成一次真实验证。

若需人工回滚，先停止 `charge-api`，恢复与旧版本兼容的数据库备份，再部署旧二进制与前端静态文件。仅移除 `WXPUSHER_APP_TOKEN` 可以关闭微信能力，不需要回滚整个应用。

## 排障

查看服务和最近日志：

```bash
sudo systemctl --no-pager --full status charge-api
sudo journalctl -u charge-api -n 100 --no-pager
```

常见启动与运行错误：

| 错误                                                 | 处理                                                                                          |
| ---------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `CHARGE_COOKIE_KEY` 无效                             | 使用 `openssl rand -base64 32` 生成，并确保没有复制换行或空格。已有数据库必须继续使用原密钥。 |
| 设置了 `WXPUSHER_APP_TOKEN` 但缺少 `PUBLIC_BASE_URL` | 配置只含协议与域名的公网 HTTPS Origin。                                                       |
| 提示公共查询通道暂无可用 Cookie                      | 前往管理端「公共池」补充可用凭据，或由用户在「平台连接」上传个人 Cookie 并加入互助公共池。     |
| 页面可打开但 SSE 不更新                              | 检查反向代理是否对 `/api/stream` 关闭缓冲并延长读取超时。                                     |
| 部署脚本提示工作区不干净                             | 提交或暂存本地改动后再部署，避免把未审阅文件带到服务器。                                      |
| 健康检查失败                                         | 查看自动输出的 systemd 状态和日志，确认前端已构建、数据库目录可写、端口未被占用。             |

返回项目概览：[README](../README.md)
