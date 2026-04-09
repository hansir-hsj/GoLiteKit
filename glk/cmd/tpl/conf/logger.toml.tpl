[logger]
dir        = "logs"
filename   = "{{.Name}}.log"
level      = "info"
format     = "text"
rotateRule = "1hour"
maxFileNum = 48
