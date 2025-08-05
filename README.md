# 充电状态监控 API

这是一个 Flask 应用，用于监控充电设备的状态，并提供一个带认证的后台来管理设备和账户。

## ✨ 功能

- **设备状态查询**: 提供 API 接口以获取充电设备各端口的实时状态（`free` 或 `busy`）。
- **管理员后台**:
    - 使用 `admin` / `password` 登录 (首次运行时会自动创建)。
    - **增删改查 (CRUD)**: 提供对设备和账户的完整管理功能。
    - 界面美化，响应式布局。
- **数据持久化**: 使用 SQLite 数据库存储所有数据。
- **Docker 部署**: 提供完整的 Docker 和 Docker Compose 部署方案，并使用 Nginx 作为反向代理。

## 🛠️ 技术栈

- **后端**: Flask, Gunicorn
- **数据库**: SQLite
- **部署**: Docker, Docker Compose, Nginx

---

## 🚀 部署指南

本文档提供了在服务器上使用 Docker 和 Docker Compose 部署此 Flask 应用的详细步骤。

### 先决条件

*   在您的服务器上已安装 Docker 和 Docker Compose。
*   您已将项目代码克隆或上传到服务器。

### 部署步骤

1.  **配置 Nginx**

    打开 `nginx.conf` 文件，并将 `your_domain.com` 替换为您的服务器域名或 IP 地址。

    ```nginx
    server {
        listen 80;
        server_name your_domain.com; # 替换为您的域名或服务器 IP
        ...
    }
    ```

2.  **创建并配置 `.env` 文件**

    项目提供了一个环境变量示例文件 `example.env`。您需要将其复制为 `.env` 文件，然后根据您的需求修改其中的值。

    **第一步：复制文件**
    ```bash
    cp example.env .env
    ```

    **第二步：修改 `.env` 文件**
    打开新创建的 `.env` 文件，并修改 `SECRET_KEY` 的值。
    ```
    # .env
    SECRET_KEY=your-super-secret-and-long-key
    ```
    **重要提示:** 请将 `your-super-secret-and-long-key` 替换为您自己的、随机生成的、足够长的密钥。

3.  **构建并启动容器**

    在项目的根目录下，运行以下命令来构建 Docker 镜像并以后台模式启动容器：

    ```bash
    docker-compose up --build -d
    ```

    *   `--build` 标志会强制重新构建镜像，以确保应用了所有最新的代码更改。
    *   `-d` 标志（detached mode）会让容器在后台运行。

4.  **验证部署**

    现在，Nginx 正在监听服务器的 80 端口，并将所有请求转发到您的 Flask 应用。您可以通过在浏览器中访问 `http://<your-server-ip>` (或您的域名) 来验证它是否正常工作。

### 管理应用

*   **查看日志:**
    ```bash
    docker-compose logs -f
    ```

*   **停止应用:**
    ```bash
    docker-compose down
    ```

*   **重启应用:**
    ```bash
    docker-compose restart
    ```

### 数据持久化

`docker-compose.yml` 文件已配置为将以下数据持久化到主机上：

*   **数据库:** `charge_status.db` 文件将映射到您主机上的 `./charge_status.db`。
*   **日志:** `logs` 目录将映射到您主机上的 `./logs` 目录。

这意味着即使您停止或移除了容器，您的数据和日志也不会丢失。