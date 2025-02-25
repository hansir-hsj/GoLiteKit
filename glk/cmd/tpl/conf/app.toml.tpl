[HttpServer]
# 应用名称
appName = "{{.App}}"
# 运行模式
runMode = "debug"
# 监听地址
addr = ":8080"

# 写超时
writeTimeout = 15000
# 读超时
readTimeout = 200
# 闲置超时
idleTimeout = 5000
# 关闭超时
shutdownTimeout = 5000

# 流速配置
[HttpServer.RateLimit]
# 常规流速限制
rateLimit = 100
# 突增流速限制
rateBurst = 150

# 日志配置
[HttpServer.Logger]
# 配置文件
configFile = "logger.toml"

# 数据库配置
[HttpServer.DB]
# 配置文件
configFile = "db.toml"