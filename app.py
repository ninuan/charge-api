from flask import Flask, render_template, jsonify, request, redirect, url_for, session, flash
from werkzeug.security import generate_password_hash, check_password_hash
from functools import wraps
from main import get_device_status
import sqlite3
from datetime import datetime
import os
import time
import logging
from logging_config import setup_logging, get_logger
from config import Config
from config import DEVICES

app = Flask(__name__)
app.config.from_object(Config)

# 设置日志
setup_logging('flask_app', debug=app.config.get('DEBUG', False))
logger = get_logger('charge_api.flask')

# 数据库配置
DATABASE = 'charge_status.db'

def init_db():
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()

    # 检查并创建所有表
    c.execute('''CREATE TABLE IF NOT EXISTS status
                 (id INTEGER PRIMARY KEY AUTOINCREMENT,
                  device_id TEXT NOT NULL,
                  port TEXT NOT NULL,
                  status TEXT NOT NULL,
                  update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP)''')
    
    c.execute('''CREATE TABLE IF NOT EXISTS admins (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    username TEXT UNIQUE NOT NULL,
                    password_hash TEXT NOT NULL
                )''')

    c.execute('''CREATE TABLE IF NOT EXISTS devices (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    logicalCode TEXT UNIQUE NOT NULL,
                    returnUrl TEXT
                )''')

    c.execute('''CREATE TABLE IF NOT EXISTS accounts (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    name TEXT UNIQUE NOT NULL,
                    cookie TEXT
                )''')

    # 检查并插入初始设备数据
    c.execute("SELECT COUNT(*) FROM devices")
    if c.fetchone()[0] == 0:
        logger.info("正在插入初始设备数据...")
        for device in DEVICES:
            c.execute("INSERT INTO devices (logicalCode, returnUrl) VALUES (?, ?)",
                      (device['logicalCode'], device['returnUrl']))
        logger.info("设备数据插入完成。")

    # 检查并插入初始账户数据
    c.execute("SELECT COUNT(*) FROM accounts")
    if c.fetchone()[0] == 0:
        from config import ACCOUNTS
        logger.info("正在插入初始账户数据...")
        for account in ACCOUNTS:
            c.execute("INSERT INTO accounts (name, cookie) VALUES (?, ?)",
                      (account['name'], account['cookie']))
        logger.info("账户数据插入完成。")

    conn.commit()
    conn.close()
    logger.info("数据库初始化完成")

def create_admin(username, password):
    """创建一个新的管理员账户"""
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    # 检查管理员是否已存在
    c.execute("SELECT id FROM admins WHERE username = ?", (username,))
    if c.fetchone() is None:
        password_hash = generate_password_hash(password)
        c.execute("INSERT INTO admins (username, password_hash) VALUES (?, ?)", (username, password_hash))
        conn.commit()
        logger.info(f"管理员 '{username}' 创建成功。")
    else:
        logger.info(f"管理员 '{username}' 已存在。")
    conn.close()


def login_required(f):
    @wraps(f)
    def decorated_function(*args, **kwargs):
        if 'admin_id' not in session:
            return redirect(url_for('admin_login', next=request.url))
        return f(*args, **kwargs)
    return decorated_function

def save_status(device_id, port, status):
    try:
        conn = sqlite3.connect(DATABASE)
        c = conn.cursor()
        c.execute("INSERT INTO status (device_id, port, status) VALUES (?, ?, ?)",
                  (device_id, port, status))
        conn.commit()
        affected_rows = c.rowcount
        conn.close()
        logger.debug(f"保存状态成功: {device_id}_{port} = {status} (影响行数: {affected_rows})")
        return True
    except Exception as e:
        logger.error(f"保存状态失败: {device_id}_{port} = {status}, 错误: {e}")
        return False

def get_last_status():
    try:
        conn = sqlite3.connect(DATABASE)
        c = conn.cursor()
        # 修改SQL查询，获取每个设备+端口组合的最新记录
        c.execute("""
            SELECT device_id, port, status, update_time 
            FROM status s1
            WHERE update_time = (
                SELECT MAX(update_time) 
                FROM status s2 
                WHERE s2.device_id = s1.device_id AND s2.port = s1.port
            )
            ORDER BY device_id, port
        """)
        rows = c.fetchall()
        conn.close()
        
        result = {}
        for row in rows:
            device_id, port, status, update_time = row
            result[f"{device_id}_{port}"] = status
        
        logger.debug(f"获取最新状态: {len(result)} 条记录")
        logger.debug(f"状态详情: {result}")
        return result
    except Exception as e:
        logger.error(f"获取最新状态失败: {e}")
        return {}

@app.route('/')
def index():
    return render_template('status.html')

@app.route('/api/status')
def get_status():
    logger.info("开始获取设备状态")
    processed_data = {}
    save_count = 0
    
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    c.execute("SELECT logicalCode FROM devices")
    devices_from_db = c.fetchall()
    conn.close()

    for device_row in devices_from_db:
        device_code = device_row[0]
        logger.info(f"正在查询设备: {device_code}")
        
        try:
            device_status = get_device_status(device_code) or {}
            
            if device_status:
                logger.info(f"设备 {device_code} 查询成功，获得 {len(device_status)} 个端口数据")
                
                # 处理并存储该设备的状态数据
                for port, info in device_status.items():
                    status = 'busy' if info.get('status') == 'busy' else 'free'
                    processed_data[f'{device_code}_{port}'] = status
                    
                    # 保存到数据库并记录结果
                    if save_status(device_code, port, status):
                        save_count += 1
                    
                logger.info(f"设备 {device_code} 已保存 {len(device_status)} 条端口数据")
            else:
                logger.warning(f"设备 {device_code} 查询失败或返回空数据")
                
        except Exception as e:
            logger.error(f"查询设备 {device_code} 时发生异常: {e}")
        
        time.sleep(1)  # 避免请求过于频繁，增加延时
    
    logger.info(f"状态查询完成，共获得 {len(processed_data)} 条数据，成功保存 {save_count} 条")
    return jsonify(processed_data)

@app.route('/api/last_status')
def get_last_status_api():
    logger.debug("获取最新状态API调用")
    return jsonify(get_last_status())

@app.route('/api/devices')
def get_devices():
    """返回当前配置的设备列表"""
    logger.debug("获取设备列表API调用")
    try:
        conn = sqlite3.connect(DATABASE)
        c = conn.cursor()
        c.execute("SELECT logicalCode, returnUrl FROM devices")
        rows = c.fetchall()
        conn.close()

        devices_info = []
        for row in rows:
            devices_info.append({
                'logicalCode': row[0],
                'returnUrl': row[1]
            })
        
        result = {
            'devices': devices_info,
            'count': len(devices_info)
        }
        logger.info(f"返回设备列表: {len(devices_info)} 个设备")
        return jsonify(result)
    except Exception as e:
        logger.error(f"获取设备列表失败: {e}")
        return jsonify({'error': str(e)}), 500

@app.route('/api/debug/all_status')
def get_all_status_debug():
    """调试用：获取所有状态记录"""
    try:
        conn = sqlite3.connect(DATABASE)
        c = conn.cursor()
        c.execute("SELECT device_id, port, status, update_time FROM status ORDER BY update_time DESC LIMIT 50")
        rows = c.fetchall()
        conn.close()
        
        result = []
        for row in rows:
            device_id, port, status, update_time = row
            result.append({
                'device_id': device_id,
                'port': port,
                'status': status,
                'update_time': update_time
            })
        
        logger.info(f"调试：返回 {len(result)} 条状态记录")
        return jsonify({
            'total_records': len(result),
            'records': result
        })
    except Exception as e:
        logger.error(f"调试API失败: {e}")
        return jsonify({'error': str(e)}), 500

@app.route('/api/debug/latest_by_device')
def get_latest_by_device_debug():
    """调试用：按设备获取最新状态"""
    try:
        conn = sqlite3.connect(DATABASE)
        c = conn.cursor()
        c.execute("""
            SELECT device_id, COUNT(*) as port_count, MAX(update_time) as latest_time
            FROM status 
            GROUP BY device_id 
            ORDER BY latest_time DESC
        """)
        rows = c.fetchall()
        conn.close()
        
        result = []
        for row in rows:
            device_id, port_count, latest_time = row
            result.append({
                'device_id': device_id,
                'port_count': port_count,
                'latest_time': latest_time
            })
        
        logger.info(f"调试：返回 {len(result)} 个设备的统计信息")
        return jsonify({
            'devices': result,
            'total_devices': len(result)
        })
    except Exception as e:
        logger.error(f"调试API失败: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/admin')
@login_required
def admin_dashboard():
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    c.execute("SELECT id, logicalCode, returnUrl FROM devices")
    devices = [{'id': row[0], 'logicalCode': row[1], 'returnUrl': row[2]} for row in c.fetchall()]
    c.execute("SELECT id, name, cookie FROM accounts")
    accounts = [{'id': row[0], 'name': row[1], 'cookie': row[2]} for row in c.fetchall()]
    conn.close()
    return render_template('admin_dashboard.html', devices=devices, accounts=accounts)

@app.route('/admin/login', methods=['GET', 'POST'])
def admin_login():
    if request.method == 'POST':
        username = request.form['username']
        password = request.form['password']
        
        conn = sqlite3.connect(DATABASE)
        c = conn.cursor()
        c.execute("SELECT id, password_hash FROM admins WHERE username = ?", (username,))
        admin = c.fetchone()
        conn.close()
        
        if admin and check_password_hash(admin[1], password):
            session['admin_id'] = admin[0]
            session['username'] = username
            flash('Login successful!', 'success')
            return redirect(url_for('admin_dashboard'))
        else:
            flash('Invalid username or password', 'danger')
            
    return render_template('admin_login.html')

@app.route('/admin/logout')
def admin_logout():
    session.pop('admin_id', None)
    session.pop('username', None)
    flash('You have been logged out.', 'info')
    return redirect(url_for('admin_login'))


@app.route('/admin/device/add', methods=['POST'])
@login_required
def add_device():
    logical_code = request.form['logicalCode']
    return_url = request.form['returnUrl']
    
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    c.execute("INSERT INTO devices (logicalCode, returnUrl) VALUES (?, ?)", (logical_code, return_url))
    conn.commit()
    conn.close()
    
    flash('Device added successfully!', 'success')
    return redirect(url_for('admin_dashboard'))

@app.route('/admin/device/delete/<int:device_id>', methods=['POST'])
@login_required
def delete_device(device_id):
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    c.execute("DELETE FROM devices WHERE id = ?", (device_id,))
    conn.commit()
    conn.close()
    
    flash('Device deleted successfully!', 'success')
    return redirect(url_for('admin_dashboard'))

@app.route('/admin/device/edit/<int:device_id>', methods=['GET', 'POST'])
@login_required
def edit_device(device_id):
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    if request.method == 'POST':
        logical_code = request.form['logicalCode']
        return_url = request.form['returnUrl']
        c.execute("UPDATE devices SET logicalCode = ?, returnUrl = ? WHERE id = ?",
                  (logical_code, return_url, device_id))
        conn.commit()
        conn.close()
        flash('Device updated successfully!', 'success')
        return redirect(url_for('admin_dashboard'))
    
    c.execute("SELECT logicalCode, returnUrl FROM devices WHERE id = ?", (device_id,))
    device_data = c.fetchone()
    conn.close()
    if device_data:
        device = {'id': device_id, 'logicalCode': device_data[0], 'returnUrl': device_data[1]}
        return render_template('edit_device.html', device=device)
    return redirect(url_for('admin_dashboard'))

@app.route('/admin/account/add', methods=['POST'])
@login_required
def add_account():
    name = request.form['name']
    cookie = request.form['cookie']
    
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    c.execute("INSERT INTO accounts (name, cookie) VALUES (?, ?)", (name, cookie))
    conn.commit()
    conn.close()
    
    flash('Account added successfully!', 'success')
    return redirect(url_for('admin_dashboard'))

@app.route('/admin/account/delete/<int:account_id>', methods=['POST'])
@login_required
def delete_account(account_id):
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    c.execute("DELETE FROM accounts WHERE id = ?", (account_id,))
    conn.commit()
    conn.close()
    
    flash('Account deleted successfully!', 'success')
    return redirect(url_for('admin_dashboard'))


@app.route('/admin/account/edit/<int:account_id>', methods=['GET', 'POST'])
@login_required
def edit_account(account_id):
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    if request.method == 'POST':
        name = request.form['name']
        cookie = request.form['cookie']
        c.execute("UPDATE accounts SET name = ?, cookie = ? WHERE id = ?",
                  (name, cookie, account_id))
        conn.commit()
        conn.close()
        flash('Account updated successfully!', 'success')
        return redirect(url_for('admin_dashboard'))

    c.execute("SELECT name, cookie FROM accounts WHERE id = ?", (account_id,))
    account_data = c.fetchone()
    conn.close()
    if account_data:
        account = {'id': account_id, 'name': account_data[0], 'cookie': account_data[1]}
        return render_template('edit_account.html', account=account)
    return redirect(url_for('admin_dashboard'))


if __name__ == '__main__':
    with app.app_context():
        init_db()
        create_admin('admin', 'password')
    app.run()