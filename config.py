# 基础配置
BASE_URL = "https://www.washpayer.com/user/message/equipmentPara"

# 公共请求头
COMMON_HEADERS = {
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
    'Priority': 'u=1, i'
}

# 设备列表
DEVICES = [
    {
        'logicalCode': 'G631085',
        'returnUrl': 'https://www.washpayer.com/user/index.html#/pages/device/selectPort?chargeIndex=&logicalCode=G631085'
    },
    {
        'logicalCode': 'G641035',
        'returnUrl': 'https://www.washpayer.com/user/index.html#/pages/device/selectPort?chargeIndex=&logicalCode=G641035'
    },
    {
        'logicalCode': 'G641014',
        'returnUrl': 'https://www.washpayer.com/user/index.html#/pages/device/selectPort?chargeIndex=&logicalCode=G641014'
    }
]

# 账户列表 (按优先级排序)
ACCOUNTS = [
    {
        'name': '账户1',
        'cookie': ''
    },
    {
        'name': '账户2',
        'cookie': ''
    }
]

import os

class Config:
    DEBUG = False
    SECRET_KEY = os.environ.get('SECRET_KEY')
    DATABASE = 'charge_status.db'
    HOST = '0.0.0.0'
    PORT = 5000