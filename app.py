from flask import Flask, render_template, jsonify
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
    if not os.path.exists(DATABASE):
        logger.info("初始化数据库")
        conn = sqlite3.connect(DATABASE)
        c = conn.cursor()
        c.execute('''CREATE TABLE IF NOT EXISTS status
                     (id INTEGER PRIMARY KEY AUTOINCREMENT,
                      device_id TEXT NOT NULL,
                      port TEXT NOT NULL,
                      status TEXT NOT NULL,
                      update_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP)''')
        conn.commit()
        conn.close()
        logger.info("数据库初始化完成")

def save_status(device_id, port, status):
    try:
        conn = sqlite3.connect(DATABASE)
        c = conn.cursor()
        c.execute("INSERT INTO status (device_id, port, status) VALUES (?, ?, ?)",
                  (device_id, port, status))
        conn.commit()
        conn.close()
        logger.debug(f"保存状态: {device_id}_{port} = {status}")
    except Exception as e:
        logger.error(f"保存状态失败: {e}")

def get_last_status():
    try:
        conn = sqlite3.connect(DATABASE)
        c = conn.cursor()
        c.execute("SELECT device_id, port, status FROM status WHERE update_time = (SELECT MAX(update_time) FROM status)")
        rows = c.fetchall()
        conn.close()
        
        result = {}
        for row in rows:
            device_id, port, status = row
            result[f"{device_id}_{port}"] = status
        
        logger.debug(f"获取最新状态: {len(result)} 条记录")
        return result
    except Exception as e:
        logger.error(f"获取最新状态失败: {e}")
        return {}

@app.route('/')
def index():
    init_db()
    return render_template('status.html')

@app.route('/api/status')
def get_status():
    logger.info("开始获取设备状态")
    processed_data = {}
    
    for device in DEVICES:
        device_code = device['logicalCode']
        logger.info(f"正在查询设备: {device_code}")
        
        try:
            device_status = get_device_status(device_code) or {}
            
            if device_status:
                logger.info(f"设备 {device_code} 查询成功，获得 {len(device_status)} 个端口数据")
                
                # 处理并存储该设备的状态数据
                for port, info in device_status.items():
                    status = 'busy' if info.get('status') == 'busy' else 'free'
                    processed_data[f'{device_code}_{port}'] = status
                    save_status(device_code, port, status)
            else:
                logger.warning(f"设备 {device_code} 查询失败或返回空数据")
                
        except Exception as e:
            logger.error(f"查询设备 {device_code} 时发生异常: {e}")
        
        time.sleep(1)  # 避免请求过于频繁，增加延时
    
    logger.info(f"状态查询完成，共获得 {len(processed_data)} 条数据")
    return jsonify(processed_data)

@app.route('/api/last_status')
def get_last_status_api():
    logger.debug("获取最新状态API调用")
    return jsonify(get_last_status())

@app.route('/api/devices')
def get_devices():
    """返回当前配置的设备列表"""
    logger.debug("获取设备列表API调用")
    devices_info = []
    for device in DEVICES:
        devices_info.append({
            'logicalCode': device['logicalCode'],
            'returnUrl': device['returnUrl']
        })
    
    result = {
        'devices': devices_info,
        'count': len(devices_info)
    }
    logger.info(f"返回设备列表: {len(devices_info)} 个设备")
    return jsonify(result)


if __name__ == '__main__':
    app.run()