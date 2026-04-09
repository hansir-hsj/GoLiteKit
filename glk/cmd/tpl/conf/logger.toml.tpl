[logger]
dir = "logs"
filename = "{{.App}}.log"
level = "trace"
format = "text"
rotateRule = "1hour"
maxFileNum = 48
