# Per-robot configuration. Copy to config.py (never commit it) and fill in.
# `make bundle CONFIG=robot/config.py` inlines it into robot/dist/main.py.

ROBOT_ID = "mbot-1"

WIFI_SSID = "lab-guest"          # 2.4 GHz only
WIFI_PASSWORD = "change-me"

BROKER_HOST = "mqtt.example.org"
BROKER_PORT = 8883               # 1883 for local development without TLS
BROKER_TLS = True
BROKER_PASSWORD = "change-me"    # user name is ROBOT_ID
