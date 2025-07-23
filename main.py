from base_request import get_ports_status
from config import BASE_URL, COMMON_HEADERS, DEVICES, ACCOUNTS
import time

def try_accounts_for_device(device):
    """尝试所有账户查询指定设备"""
    for account in ACCOUNTS:
        # 准备请求参数
        params = {
            'logicalCode': device['logicalCode'],
            'returnUrl': device['returnUrl'],
            'refresh': 'false'
        }
        
        # 准备请求头
        headers = COMMON_HEADERS.copy()
        headers['Cookie'] = account['cookie']
        
        # 执行请求
        print(f"\n尝试使用 {account['name']} 查询设备 {device['logicalCode']}...")
        charge_index = get_ports_status(BASE_URL, params, headers)
        
        if charge_index:
            print(f"设备 {device['logicalCode']} 查询成功 (使用 {account['name']})")
            return True  # 查询成功则停止尝试其他账户
        
        print(f"设备 {device['logicalCode']} 查询失败 (使用 {account['name']})")
        time.sleep(3)  # 失败后等待3秒再尝试下一个账户
    
    return False  # 所有账户都尝试失败

def get_device_status(device_logical_code):
    """获取指定设备的状态"""
    # 找到对应的设备配置
    target_device = None
    for device in DEVICES:
        if device['logicalCode'] == device_logical_code:
            target_device = device
            break
    
    if not target_device:
        print(f"未找到设备 {device_logical_code} 的配置")
        return None
    
    # 尝试所有账户查询该设备
    for account in ACCOUNTS:
        # 准备请求参数
        params = {
            'logicalCode': target_device['logicalCode'],
            'returnUrl': target_device['returnUrl'],
            'refresh': 'false'
        }
        
        # 准备请求头
        headers = COMMON_HEADERS.copy()
        headers['Cookie'] = account['cookie']
        
        # 执行请求
        charge_index = get_ports_status(BASE_URL, params, headers)
        
        if charge_index:
            return charge_index
        
        time.sleep(1)  # 失败后等待1秒再尝试下一个账户
    
    return None  # 所有账户都尝试失败

def poll_devices():
    """轮询所有设备"""
    for device in DEVICES:
        success = try_accounts_for_device(device)
        if not success:
            print(f"警告: 设备 {device['logicalCode']} 所有账户尝试失败")
        time.sleep(5)  # 设备间查询间隔

if __name__ == "__main__":
    while True:
        print("\n=== 开始新一轮设备查询 ===")
        poll_devices()
        print("\n=== 本轮查询完成 ===")
        time.sleep(30)  # 每轮查询间隔30秒