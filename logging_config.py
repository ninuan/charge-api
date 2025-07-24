import logging
import logging.handlers
import os

def setup_logging(app_name='charge_api', debug=False):
    """
    设置统一的日志配置
    """
    # 确保logs目录存在
    if not os.path.exists('logs'):
        os.makedirs('logs')
    
    # 根据应用名称设置日志文件
    log_file = f'logs/{app_name}.log'
    
    # 配置根日志记录器
    root_logger = logging.getLogger()
    root_logger.setLevel(logging.DEBUG if debug else logging.INFO)
    
    # 清除现有的处理器（避免重复）
    root_logger.handlers.clear()
    
    # 文件处理器 - 轮转日志
    file_handler = logging.handlers.RotatingFileHandler(
        log_file, 
        maxBytes=10*1024*1024,  # 10MB
        backupCount=5,
        encoding='utf-8'
    )
    file_formatter = logging.Formatter(
        '%(asctime)s %(name)s %(levelname)s: %(message)s [%(filename)s:%(lineno)d]'
    )
    file_handler.setFormatter(file_formatter)
    file_handler.setLevel(logging.DEBUG if debug else logging.INFO)
    
    # 控制台处理器
    console_handler = logging.StreamHandler()
    console_formatter = logging.Formatter(
        '%(asctime)s %(levelname)s: %(message)s'
    )
    console_handler.setFormatter(console_formatter)
    console_handler.setLevel(logging.INFO)
    
    # 添加处理器到根日志记录器
    root_logger.addHandler(file_handler)
    if debug:
        root_logger.addHandler(console_handler)
    
    # 配置特定模块的日志级别
    logging.getLogger('charge_api.requests').setLevel(logging.DEBUG if debug else logging.INFO)
    logging.getLogger('charge_api.main').setLevel(logging.DEBUG if debug else logging.INFO)
    logging.getLogger('werkzeug').setLevel(logging.WARNING)  # 减少Flask默认日志
    
    return root_logger

def get_logger(name):
    """
    获取指定名称的日志记录器
    """
    return logging.getLogger(name)
