[HttpServer]
appName  = "{{.Name}}"
runMode  = "debug"
addr     = ":8080"
# set to true to enable pprof endpoints
enablePprof = false

# timeout values in milliseconds
[HttpServer.Timeout]
writeTimeout      = 15000
readTimeout       = 15000
readHeaderTimeout = 5000
idleTimeout       = 60000
shutdownTimeout   = 5000

# SSE requests (Accept: text/event-stream) use this handler and write timeout
# instead of writeTimeout, in milliseconds
[HttpServer.SSE]
timeout = 300000

# Paths below are relative to this file's directory; absolute paths are used as is.

# logger config file path
[HttpServer.Logger]
configFile = "logger.toml"

# database config file path, used by db.NewFromConfig()
[HttpServer.DB]
configFile = "db.toml"

# redis config file path, used by redis.NewFromConfig()
[HttpServer.Redis]
configFile = "redis.toml"
