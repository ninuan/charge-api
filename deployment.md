# 充电状态监控系统部署指南

## 服务器环境准备
1. 安装Python 3.8+
2. 安装Nginx
3. 安装SQLite3

## Nginx配置说明
nginx.conf文件是Nginx服务器的核心配置文件，主要作用：
- 监听80端口处理HTTP请求
- 将请求反向代理到Gunicorn服务(127.0.0.1:8000)
- 设置必要的请求头(X-Real-IP等)
- 可配置静态文件服务

关键配置项说明：
- `listen 80`: 监听80端口
- `server_name`: 替换为您的域名
- `proxy_pass`: 指向Gunicorn服务地址
- `location /static/`: 静态文件目录(需配置实际路径)

## 应用部署
1. 将项目文件上传至服务器 `/opt/charge-status` 目录
2. 创建虚拟环境：
   ```bash
   python -m venv venv
   source venv/bin/activate
   pip install -r requirements.txt
   ```

3. 配置Nginx：
   - 复制nginx.conf到 `/etc/nginx/sites-available/charge-status`
   - 创建符号链接：
     ```bash
     sudo ln -s /etc/nginx/sites-available/charge-status /etc/nginx/sites-enabled
     ```
   - 测试并重启Nginx：
     ```bash
     sudo nginx -t
     sudo systemctl restart nginx
     ```

4. 启动应用：
   ```bash
   chmod +x start.sh
   ./start.sh
   ```

5. 使用进程管理(可选)：
   - 使用systemd或supervisor管理Gunicorn进程

## 环境变量配置
建议设置SECRET_KEY环境变量：
```bash
export SECRET_KEY='your-production-secret-key'