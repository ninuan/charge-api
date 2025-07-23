import requests
import json

def get_ports_status(base_url, params, headers):
    """
    发送请求并获取充电端口状态
    """
    try:
        print(f"正在向服务器发送请求，查询设备 {params['logicalCode']}...")
        response = requests.get(base_url, params=params, headers=headers)
        
        response.raise_for_status()
        print("请求成功！状态码:", response.status_code)
        
        data = response.json()
        
        if data.get('result') != 1:
            error_description = data.get('description', '未知错误')
            print(f"请求失败！服务器返回错误: {error_description}")
            print("这通常意味着你的 'MyUser_session_id' Cookie 已过期，需要重新抓包更新。")
            return None
            
        charge_index = data.get('payload', {}).get('devType', {}).get('chargeIndex', {})
        
        if not charge_index:
            print("错误：在响应中未找到 'chargeIndex' 数据。")
            return

        print(f"\n--- 充电桩 ({params['logicalCode']}) 端口状态 ---")
        ports = sorted(charge_index.items(), key=lambda item: int(item[0]))
        
        for port_number, port_info in ports:
            status = port_info.get('status', '未知')
            status_text = "繁忙 (充电中)" if status == 'busy' else "空闲"
        
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