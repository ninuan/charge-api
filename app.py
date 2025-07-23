from flask import Flask, render_template, jsonify
import request2
import request
import sqlite3
from datetime import datetime
import os

app = Flask(__name__)

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
    # 获取两个设备的端口状态
    status1 = request2.get_ports_status() or {}
    status2 = request.get_ports_status() or {}
    
    # 处理并存储状态数据
    processed_data = {}
    for port, info in status1.items():
        status = 'busy' if info.get('status') == 'busy' else 'free'
        processed_data[f'G631085_{port}'] = status
        save_status('G631085', port, status)
    
    for port, info in status2.items():
        status = 'busy' if info.get('status') == 'busy' else 'free'
        processed_data[f'G641035_{port}'] = status
        save_status('G641035', port, status)
    
    return jsonify(processed_data)

@app.route('/api/last_status')
def get_last_status_api():
    return jsonify(get_last_status())


if __name__ == '__main__':
    app.run(debug=True)