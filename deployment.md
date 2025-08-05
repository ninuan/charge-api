# Docker 部署指南

本文档提供了在服务器上使用 Docker 和 Docker Compose 部署此 Flask 应用的详细步骤。

## 先决条件

*   在您的服务器上已安装 Docker 和 Docker Compose。
*   您已将项目代码克隆或上传到服务器。

## 部署步骤

1.  **创建 `.env` 文件**

    在项目的根目录下，创建一个名为 `.env` 的文件。这个文件将用于存储敏感信息，如 `SECRET_KEY`。

    ```
    SECRET_KEY=your-super-secret-and-long-key
    ```

    **重要提示:** 请将 `your-super-secret-and-long-key` 替换为您自己的、随机生成的、足够长的密钥。

2.  **构建并启动容器**

    在项目的根目录下，运行以下命令来构建 Docker 镜像并以后台模式启动容器：

    ```bash
    docker-compose up --build -d
    ```

    *   `--build` 标志会强制重新构建镜像，以确保应用了所有最新的代码更改。
    *   `-d` 标志（detached mode）会让容器在后台运行。

3.  **验证部署**

    应用现在应该正在运行，并监听服务器的 5000 端口。您可以通过在浏览器中访问 `http://<your-server-ip>:5000` 来验证它是否正常工作。

## 管理应用

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

## 数据持久化

`docker-compose.yml` 文件已配置为将以下数据持久化到主机上：

*   **数据库:** `charge_status.db` 文件将映射到您主机上的 `./charge_status.db`。
*   **日志:** `logs` 目录将映射到您主机上的 `./logs` 目录。

这意味着即使您停止或移除了容器，您的数据和日志也不会丢失。