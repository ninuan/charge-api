import requests
import json

# ==================== 配置区 (根据新抓包内容更新) ====================

# 1. 定义请求的目标URL和参数
# base_url 保持不变
base_url = "https://www.washpayer.com/user/message/equipmentPara"

# G641035 是你新抓包中的设备ID
params = {
    'logicalCode': 'G641035',
    'returnUrl': 'https://www.washpayer.com/user/index.html#/pages/device/selectPort?chargeIndex=&logicalCode=G641035',
    'refresh': 'false'
}

# 2. 定义请求头 (Headers)，这是最关键的部分
# 直接从你最新的抓包结果中复制过来
headers = {
    'Host': 'www.washpayer.com',
    'Platform': 'h5',
    'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 NetType/WIFI MicroMessenger/7.0.20.1781(0x6700143B) WindowsWechat(0x63090a13) UnifiedPCWindowsWechat(0xf2540615) XWEB/14199 Flue',
    'Accept': '*/*',
    'Sec-Fetch-Site': 'same-origin',
    'Sec-Fetch-Mode': 'cors',
    'Sec-Fetch-Dest': 'empty',
    'Referer': 'https://www.washpayer.com/user/index.html',
    'Accept-Encoding': 'gzip, deflate, br',
    'Accept-Language': 'zh-CN,zh;q=0.9',
    # 将最新抓包的所有Cookie合并到一个字符串中，用分号和空格分隔
    # !!! 核心认证信息，特别是 MyUser_session_id 已经更新 !!!
    'Cookie': 'jwt_auth_domain=MyUser; agentLogoUrl="aHR0cHM6Ly9yZXNvdXJjZS53YXNocGF5ZXIuY29tL3VwbG9hZGVkL2xvZ28vOTY1YjkzYjJmNDRiMjZkMzk2ZjFmNzJiZDE5MWUxMjEuanBlZw=="; agentId=5d857a130030483f797808b5; agentBrandName=%E6%98%8C%E5%8E%9F%E4%BA%91%E5%85%85; gdt_fp=85291b4d95f97716d32df25995e93e41; user_dev_no=861290073863818; MyUser_session_id=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpYXQiOjE3NTMyNTgzOTcsInVzZXJfaWQiOiI2ODZjOGEzNzZmMjkyNTQzOTk0OGQ4YzgiLCJleHAiOjE3NTU4NzkxOTd9.2UTDp74-sXidKesoqDvr83lYwfUJ7d--tFpigV54Nfo',
    'Priority': 'u=1, i'
}

# ==================== 功能实现区 (保持原有样式) ====================

def get_ports_status():
    """
    发送请求并获取充电端口状态
    """
    try:
        # 3. 发送GET请求
        print(f"正在向服务器发送请求，查询设备 {params['logicalCode']} ...")
        response = requests.get(base_url, params=params, headers=headers)
        
        # 检查响应状态码
        response.raise_for_status()  # 如果状态码不是200-299，会抛出异常
        
        print("请求成功！状态码:", response.status_code)
        
        # 4. 解析返回的JSON数据
        data = response.json()
        
        # 检查业务层面的返回结果
        if data.get('result') != 1:
            error_description = data.get('description', '未知错误')
            print(f"请求失败！服务器返回错误: {error_description}")
            print("这通常意味着你的 'MyUser_session_id' Cookie 已过期，需要重新抓包更新。")
            return None
        
        # 提取核心数据
        charge_index = data.get('payload', {}).get('devType', {}).get('chargeIndex', {})
        
        if not charge_index:
            print("错误：在响应中未找到 'chargeIndex' 数据。")
            return

        # 5. 格式化并打印端口状态
        print(f"\n--- 充电桩 ({params['logicalCode']}) 端口状态 ---")
        
        # 为了按端口号排序显示，我们先处理一下数据
        ports = sorted(charge_index.items(), key=lambda item: int(item[0]))

        for port_number, port_info in ports:
            status = port_info.get('status', '未知')
            # 根据状态进行翻译
            status_text = "繁忙 (充电中)" if status == 'busy' else "空闲"
            # print(f"端口 {port_number.rjust(2)}: {status_text}")
        
        # print("---------------------------------------")
        
        return charge_index
        
    except requests.exceptions.HTTPError as e:
        print(f"请求失败，HTTP错误: {e}")
        if e.response.status_code in [401, 403]:
            print("认证失败！请检查你的 Cookie 是否正确或已过期。")
    except requests.exceptions.RequestException as e:
        print(f"请求失败，发生网络错误: {e}")
    except json.JSONDecodeError:
        print("请求失败，无法解析服务器返回的JSON数据。")
        print("服务器返回内容:", response.text)
    except Exception as e:
        print(f"处理过程中发生未知错误: {e}")


# --- 主程序入口 ---
if __name__ == "__main__":
    get_ports_status()
