import requests
import json
import logging

# 配置独立的请求日志记录器
logger = logging.getLogger('charge_api.requests')

def get_ports_status(base_url, params, headers):
    """
    发送请求并获取充电端口状态
    """
    try:
        logger.info(f"正在向服务器发送请求，查询设备 {params['logicalCode']}...")
        response = requests.get(base_url, params=params, headers=headers, timeout=30)
        
        logger.debug(f"响应状态码: {response.status_code}")
        logger.debug(f"响应头: {dict(response.headers)}")
        logger.debug(f"响应内容长度: {len(response.text)}")
        logger.debug(f"响应内容前500字符: {response.text[:500]}")
        
        response.raise_for_status()
        logger.info("请求成功！状态码: %d", response.status_code)
        
        # 检查响应内容是否为空
        if not response.text.strip():
            logger.error("错误：服务器返回空响应")
            return None
            
        # 检查是否是 HTML 响应（通常是错误页面）
        if response.text.strip().startswith('<'):
            logger.error("错误：服务器返回 HTML 内容而不是 JSON")
            logger.debug("HTML 内容: %s", response.text[:1000])
            return None
        
        try:
            data = response.json()
        except json.JSONDecodeError as e:
            logger.error(f"JSON 解析失败: {e}")
            logger.debug(f"响应内容: {response.text}")
            return None
        
        if data.get('result') != 1:
            error_description = data.get('description', '未知错误')
            logger.error(f"请求失败！服务器返回错误: {error_description}")
            logger.warning("这通常意味着你的 'MyUser_session_id' Cookie 已过期，需要重新抓包更新。")
            return None
            
        charge_index = data.get('payload', {}).get('devType', {}).get('chargeIndex', {})
        
        if not charge_index:
            logger.error("错误：在响应中未找到 'chargeIndex' 数据。")
            return None

        logger.info(f"充电桩 ({params['logicalCode']}) 端口状态查询成功")
        ports = sorted(charge_index.items(), key=lambda item: int(item[0]))
        
        for port_number, port_info in ports:
            status = port_info.get('status', '未知')
            status_text = "繁忙 (充电中)" if status == 'busy' else "空闲"
            logger.debug(f"端口 {port_number}: {status_text}")
        
        return charge_index
        
    except requests.exceptions.Timeout:
        logger.error("请求超时，请检查网络连接")
        return None
    except requests.exceptions.HTTPError as e:
        logger.error(f"请求失败，HTTP错误: {e}")
        if e.response.status_code in [401, 403]:
            logger.error("认证失败！请检查你的 Cookie 是否正确或已过期。")
        elif e.response.status_code == 500:
            logger.error("服务器内部错误")
        return None
    except requests.exceptions.RequestException as e:
        logger.error(f"请求失败，发生网络错误: {e}")
        return None
    except json.JSONDecodeError:
        logger.error("请求失败，无法解析服务器返回的JSON数据。")
        logger.debug("服务器返回内容: %s", response.text)
        return None
    except Exception as e:
        logger.error(f"处理过程中发生未知错误: {e}")
        return None