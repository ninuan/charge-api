import os

class Config:
    DEBUG = False
    SECRET_KEY = os.environ.get('SECRET_KEY') or 'your-secret-key-here'
    DATABASE = 'charge_status.db'
    HOST = '0.0.0.0'
    PORT = 5000