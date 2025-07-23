import requests
import json

# 1. 定义请求的目标URL和参数
# 我将原始URL中的参数部分拆分出来，这样更清晰
base_url = "https://www.washpayer.com/user/message/equipmentPara"
params = {
    'logicalCode': 'G631085',
    'returnUrl': 'https://www.washpayer.com/user/index.html#/pages/device/selectPort?chargeIndex=&logicalCode=G631085',
    'refresh': 'false'
}

# 2. 定义请求头 (Headers)，这是最关键的部分
# 直接从你的抓包结果中复制过来
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
    # 将抓到的所有Cookie合并到一个字符串中，用分号和空格分隔
    'Cookie': 'gdt_fp=85291b4d95f97716d32df25995e93e41; jwt_auth_domain=MyUser; user_dev_no=868327078809959; MyUser_session_id=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpYXQiOjE3NTMyNTY3NzAsInVzZXJfaWQiOiI2ODZjOGEzNzZmMjkyNTQzOTk0OGQ4YzgiLCJleHAiOjE3NTU4Nzc1NzB9.jJTNkHfzNkUOHHHpw0ZsOZBfc67vf2Ap476IaYZlMso',
    'Priority': 'u=1, i'
}

def get_ports_status():
    """
    发送请求并获取充电端口状态
    """
    try:
        # 3. 发送GET请求
        print("正在向服务器发送请求...")
        response = requests.get(base_url, params=params, headers=headers)
        
        # 检查响应状态码
        response.raise_for_status()  # 如果状态码不是200-299，会抛出异常
        
        print("请求成功！状态码:", response.status_code)
        
        # 4. 解析返回的JSON数据
        data = response.json()
        
        # 提取核心数据
        charge_index = data.get('payload', {}).get('devType', {}).get('chargeIndex', {})
        
        if not charge_index:
            print("错误：在响应中未找到 'chargeIndex' 数据。")
            # 打印完整的响应内容以供调试
            # print("完整响应内容:", json.dumps(data, indent=2, ensure_ascii=False))
            return

        # 5. 格式化并打印端口状态
        print("\n--- 充电桩 (G631085) 端口状态 ---")
        
        # 为了按端口号排序显示，我们先处理一下数据
        ports = sorted(charge_index.items(), key=lambda item: int(item[0]))

        for port_number, port_info in ports:
            status = port_info.get('status', '未知')
            # 根据状态进行翻译
            status_text = "繁忙 (充电中)" if status == 'busy' else "空闲"
            # print(f"端口 {port_number.rjust(2)}: {status_text}")
        
        # print("---------------------------------")
        
        return charge_index

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
