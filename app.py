from flask import Flask, render_template, jsonify
from main import get_device_status
from config import Config, DEVICES
import sqlite3
from datetime import datetime
import os

app = Flask(__name__)
app.config.from_object(Config)

# 数据库配置
DATABASE = 'charge_status.db'

def init_db():
    if not os.path.exists(DATABASE):
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

def save_status(device_id, port, status):
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    c.execute("INSERT INTO status (device_id, port, status) VALUES (?, ?, ?)",
              (device_id, port, status))
    conn.commit()
    conn.close()

def get_last_status():
    conn = sqlite3.connect(DATABASE)
    c = conn.cursor()
    c.execute("SELECT device_id, port, status FROM status WHERE update_time = (SELECT MAX(update_time) FROM status)")
    rows = c.fetchall()
    conn.close()
    
    result = {}
    for row in rows:
        device_id, port, status = row
        result[f"{device_id}_{port}"] = status
    return result

@app.route('/')
def index():
    init_db()
    return render_template('status.html')

@app.route('/api/status')
def get_status():
    # 动态获取所有设备的端口状态
    processed_data = {}
    
    for device in DEVICES:
        device_code = device['logicalCode']
        device_status = get_device_status(device_code) or {}
        
        # 处理并存储该设备的状态数据
        for port, info in device_status.items():
            status = 'busy' if info.get('status') == 'busy' else 'free'
            processed_data[f'{device_code}_{port}'] = status
            save_status(device_code, port, status)
    
    return jsonify(processed_data)

@app.route('/api/last_status')
def get_last_status_api():
    return jsonify(get_last_status())

@app.route('/api/devices')
def get_devices():
    """返回当前配置的设备列表"""
    devices_info = []
    for device in DEVICES:
        devices_info.append({
            'logicalCode': device['logicalCode'],
            'returnUrl': device['returnUrl']
        })
    return jsonify({
        'devices': devices_info,
        'count': len(devices_info)
    })


if __name__ == '__main__':
    app.run()