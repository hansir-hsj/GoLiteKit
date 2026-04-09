[HttpServer]
appName  = "{{.Name}}"
runMode  = "debug"
addr     = ":8080"
# set to true to enable pprof endpoints
enablePprof = false

# timeout values in milliseconds
[HttpServer.Timeout]
writeTimeout    = 15000
readTimeout     = 200
idleTimeout     = 5000
shutdownTimeout = 5000

# rate limiting
[HttpServer.RateLimit]
rateLimit = 100
rateBurst = 150

# logger config file path (relative to working directory)
[HttpServer.Logger]
configFile = "logger.toml"

# database config file path
[HttpServer.DB]
configFile = "db.toml"

# redis config file path
[HttpServer.Redis]
configFile = "redis.toml"
