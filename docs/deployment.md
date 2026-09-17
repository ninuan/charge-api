# Charge Console 生产部署与运维指南

本文面向使用 Linux、systemd 和反向代理的自托管部署。首次安装、升级、密钥、备份和排障都以仓库当前提供的脚本与服务模板为准。

> [!IMPORTANT]
> 升级已有实例前先备份 SQLite，并保存与数据库匹配的加密密钥。不要让旧版本程序直接打开已经由新版本升级过的数据库。

## 升级至 1.5.4

- 前后端必须同时更新到 1.5.4；首次启动将 SQLite schema 从 12 增量升级到 13，新增公告、内容修订与账号确认记录，不删除原有业务数据。
- 先用现有备份流程保存数据库及匹配密钥，再部署。回滚应停服后同时恢复旧版程序、前端与升级前数据库；恢复旧备份会丢失升级后新写入的数据。不要让旧程序直接打开 schema 13 数据库。
- 更新后检查 `/healthz` 的版本为 1.5.4，并用管理员进入“公告管理”。新库默认没有公告，升级不会自动给用户发布消息。
- 定时展示与结束按服务端时间计算，不需要新增 cron。管理员编辑时间采用北京时间，API 使用带时区的时间。
- 公告确认状态在服务端按账号保存。普通公告确认后收起，重要公告在有效期内保留摘要；撤下后用户不可再访问，已结束公告可回看。
- 公告接口及个性化状态不得配置 CDN 共享缓存；静态 hash 资源的缓存策略沿用原配置。
- 上线后补测实际网络下的冷/暖加载、登录后 API 耗时；本地资源变化不能直接当作线上提速比例。本版未改变 Cloudflare 接入或扫码服务部署。

## 推荐拓扑

```text
Internet
   │ HTTPS
Nginx / Caddy / Cloudflare
   │ http://127.0.0.1:8080
charge-server
   ├── /var/lib/charge-api/charge_state.db
   ├── http://127.0.0.1:8000 → yyb_go（可选）
   ├── 远端充电服务
   └── WxPusher HTTPS API（可选）
```

基本边界：

- 只有 HTTPS 反向代理对公网开放。
- `charge-server` 监听 `127.0.0.1:8080`。
- `yyb_go` 监听 `127.0.0.1:8000`，不允许公网访问。
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

### 3. 配置环境变量

先生成密钥：

```bash
cd /opt/charge-api
./scripts/gen_secrets.sh
```

将实际值写入 `/etc/charge-api.env`：

```env
CHARGE_ADMIN_PASSWORD=replace-with-a-strong-initial-password
CHARGE_COOKIE_KEY=replace-with-base64-encoded-32-byte-key

# 可选：扫码登录与凭据恢复
YYB_BASE_URL=http://127.0.0.1:8000
YYB_API_SECRET=replace-with-shared-hmac-secret

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
- `CHARGE_COOKIE_KEY` 用于加密 Cookie、WxPusher UID 等敏感数据，必须长期保存。
- `PUBLIC_BASE_URL` 必须是 HTTPS Origin，例如 `https://charge.example.com`；不能包含路径、查询参数、凭据或结尾斜杠。
- 生产环境不要设置 `WXPUSHER_BASE_URL`，让程序使用 WxPusher 官方 HTTPS API。
- AppToken 只能放在服务端环境文件中，不能使用 `NEXT_PUBLIC_*` 或提交到 Git。

### 4. 可选：配置 yyb_go

如果需要扫码登录和凭据自动恢复，`yyb_go` 应安装在 `/opt/yyb_go` 并读取 `/etc/yyb-go.env`：

```env
YYB_SECRET_KEY=replace-with-base64-encoded-32-byte-key
YYB_API_SECRET=replace-with-the-same-secret-used-by-charge
```

其中两边的 `YYB_API_SECRET` 必须完全一致。设置权限：

```bash
sudo chown root:yyb /etc/yyb-go.env
sudo chmod 0600 /etc/yyb-go.env
```

`YYB_SECRET_KEY` 用于加密 sidecar 数据库中的登录状态；更换后既有数据无法解密。

### 5. 安装 systemd 服务

仓库提供：

```text
deploy/systemd/charge.service
deploy/systemd/yyb-go.service
deploy/systemd/charge-backup.service
deploy/systemd/charge-backup.timer
```

安装 Charge 服务：

```bash
sudo cp /opt/charge-api/deploy/systemd/charge.service /etc/systemd/system/charge-api.service
sudo systemctl daemon-reload
sudo systemctl enable --now charge-api
```

启用了 `yyb_go` 时再安装 sidecar：

```bash
sudo cp /opt/charge-api/deploy/systemd/yyb-go.service /etc/systemd/system/yyb-go.service
sudo systemctl daemon-reload
sudo systemctl enable --now yyb-go
sudo systemctl restart charge-api
```

sidecar 模板会执行：

```bash
/opt/yyb_go/yyb-go \
  -host 127.0.0.1 \
  -port 8000 \
  -resource-root /opt/yyb_go/resource \
  -db yyb.db
```

模板中的 Charge 启动命令为：

```bash
/opt/charge-api/backend/charge-server \
  -listen 127.0.0.1:8080 \
  -database /var/lib/charge-api/charge_state.db \
  -state /var/lib/charge-api/charge_state.json
```

服务模板启用了 `NoNewPrivileges`、`PrivateTmp`、`ProtectSystem=strict` 和 `UMask=0077`；Charge 只允许写入 `/var/lib/charge-api`。

### 6. 配置反向代理

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

### 7. 验证服务

```bash
curl -fsS http://127.0.0.1:8080/healthz
sudo systemctl status charge-api --no-pager
sudo journalctl -u charge-api -n 100 --no-pager
```

启用 WxPusher 后，再由普通用户在通知中心完成“获取二维码 → 微信扫码 → 发送测试消息”。“WxPusher 已处理”表示供应商完成处理，不等同于某台微信客户端已经展示或已读。

## 环境变量

### Charge

| 变量                    | 默认值   | 说明                                      |
| ----------------------- | -------- | ----------------------------------------- |
| `CHARGE_ADMIN_PASSWORD` | 自动生成 | 首次初始化管理员密码。                    |
| `CHARGE_COOKIE_KEY`     | 无       | 32 字节 Base64 加密密钥；生产必需。       |
| `CORS_ALLOWED_ORIGINS`  | 同源     | 逗号分隔的允许 Origin。                   |
| `YYB_BASE_URL`          | 关闭     | `yyb_go` 地址。                           |
| `YYB_API_SECRET`        | 无       | 启用 `YYB_BASE_URL` 时必需。              |
| `MOCELE_BASE_URL`       | 内置默认 | 可选自动登录服务覆盖地址。                |
| `MOCELE_ORG`            | 内置默认 | 可选组织参数。                            |
| `MOCELE_OPENINDEX`      | 内置默认 | 可选入口参数。                            |
| `WXPUSHER_APP_TOKEN`    | 关闭     | WxPusher 服务端 AppToken。                |
| `PUBLIC_BASE_URL`       | 无       | 启用 WxPusher 时必需的公网 HTTPS Origin。 |
| `WXPUSHER_BASE_URL`     | 官方 API | 仅用于本地 mock 或受控私有代理。          |

### yyb_go

| 变量             | 说明                              |
| ---------------- | --------------------------------- |
| `YYB_SECRET_KEY` | sidecar 敏感状态的 AES-GCM 密钥。 |
| `YYB_API_SECRET` | 与 Charge 相同的 HMAC 共享密钥。  |

### 备份服务

`charge-backup.service` 可从 `/etc/charge-backup.env` 读取覆盖项。常见变量包括备份目录、数据库路径和保留天数；可直接查看 [`scripts/backup_db.sh`](../scripts/backup_db.sh) 中的默认值。

## 更新部署

### 推荐：Git 同步部署

在开发机仓库运行：

```bash
make deploy-git DEPLOY_HOST=root@<服务器IP>
```

脚本会：

1. 确认本地工作区没有未提交修改。
2. 运行 `make check`。
3. 将当前分支推送到 GitHub。
4. 通过 SSH 在服务器执行 `git pull --ff-only`。
5. 安装前端依赖并构建静态页面。
6. 构建新的 Go 二进制并原子替换。
7. 重启 systemd 服务并轮询 `/healthz`。

自定义服务器路径、分支和服务名：

```bash
make deploy-git \
  DEPLOY_HOST=root@<服务器IP> \
  DEPLOY_PATH=/opt/charge-api \
  DEPLOY_BRANCH=main \
  SERVICE_NAME=charge-api
```

服务器访问 npm 官方源不稳定时：

```bash
NPM_REGISTRY=https://registry.npmmirror.com \
PNPM_FETCH_TIMEOUT=180000 \
PNPM_FETCH_RETRIES=8 \
make deploy-git DEPLOY_HOST=root@<服务器IP>
```

预演命令，不推送也不连接服务器：

```bash
SKIP_CHECK=1 make deploy-git \
  DEPLOY_HOST=root@<服务器IP> \
  DEPLOY_ARGS=--dry-run
```

### 备用：rsync 直传

GitHub 临时不可用时：

```bash
make deploy DEPLOY_HOST=root@<服务器IP>
```

该脚本会排除 `.local/`、`.env`、SQLite、Cookie 密钥、`node_modules/` 和本地构建目录等运行数据。

## 备份与恢复

Charge 和可选 sidecar 的数据库分别为：

```text
/var/lib/charge-api/charge_state.db
/opt/yyb_go/resource/db/yyb.db
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

默认每天 03:30 执行，并随机延迟最多 15 分钟；备份写入 `/var/lib/charge-api/backups/`，使用 `0600` 权限并滚动保留 14 天。

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

备份也包含敏感状态，应加密异地保存并限制权限。恢复清单必须同时包含原始的 `CHARGE_COOKIE_KEY`、`YYB_SECRET_KEY` 和 `YYB_API_SECRET`。

## 安全加固

### 检查监听地址

```bash
ss -lntp | grep -E ':8080|:8000'
```

预期只看到 `127.0.0.1:8080` 和可选的 `127.0.0.1:8000`。不应出现 `0.0.0.0:8000`。防火墙也应阻止 sidecar 端口：

```bash
sudo ufw deny 8000/tcp
sudo ufw status
```

### 检查文件权限

```bash
stat -c '%a %n' \
  /etc/charge-api.env \
  /etc/yyb-go.env \
  /var/lib/charge-api/charge_state.db \
  /opt/yyb_go/resource/db/yyb.db
```

环境文件和数据库推荐为 `0600`。如果某个可选文件不存在，可以从命令中移除。

### 运行端到端安全检查

```bash
cd /opt/charge-api
./scripts/security_check.sh
```

首次可先预演：

```bash
./scripts/security_check.sh --dry-run
```

服务器路径不同时可以覆盖：

```bash
CHARGE_DB_FILE=/path/to/charge_state.db \
YYB_DB_FILE=/path/to/yyb.db \
LOG_UNITS="charge-api yyb-go" \
./scripts/security_check.sh
```

该检查用于确认 sidecar 未暴露、未签名请求被拒绝，以及数据库和日志没有出现已知明文敏感值。

## 升级与回滚

升级前：

1. 创建在线备份并执行 `PRAGMA integrity_check;`。
2. 保存当前二进制、Git 提交号和数据库 Schema 对应的发布说明。
3. 确认所有加密密钥已有离线副本。
4. 阅读目标版本的 [`RELEASE_NOTES.md`](../RELEASE_NOTES.md) 和 [`CHANGELOG.md`](../CHANGELOG.md)。

部署后：

1. `/healthz` 返回预期版本。
2. 普通用户可以登录、加载看板并主动刷新。
3. 管理员运维页没有新的连续异常。
4. 已启用的扫码登录、临时提醒和 WxPusher 分别完成一次真实验证。

回滚时先停止 `charge-api`，恢复与旧版本兼容的数据库备份，再部署旧二进制。仅移除 `WXPUSHER_APP_TOKEN` 可以关闭微信能力，不需要回滚整个应用。

## 排障

查看服务和最近日志：

```bash
sudo systemctl --no-pager --full status charge-api
sudo journalctl -u charge-api -n 100 --no-pager
```

常见启动错误：

| 错误                                                 | 处理                                                                                          |
| ---------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `CHARGE_COOKIE_KEY` 无效                             | 使用 `openssl rand -base64 32` 生成，并确保没有复制换行或空格。已有数据库必须继续使用原密钥。 |
| 设置了 `YYB_BASE_URL` 但缺少 `YYB_API_SECRET`        | 在 Charge 和 sidecar 环境文件中填入相同共享密钥。                                             |
| 设置了 `WXPUSHER_APP_TOKEN` 但缺少 `PUBLIC_BASE_URL` | 配置只含协议与域名的公网 HTTPS Origin。                                                       |
| 页面可打开但 SSE 不更新                              | 检查反向代理是否对 `/api/stream` 关闭缓冲并延长读取超时。                                     |
| 部署脚本提示工作区不干净                             | 提交或暂存本地改动后再部署，避免把未审阅文件带到服务器。                                      |
| 健康检查失败                                         | 查看 systemd 状态和日志，确认前端已构建、数据库目录可写、端口未被占用。                       |

返回项目概览：[README](../README.md)
