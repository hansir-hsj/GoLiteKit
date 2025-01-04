# golite

简洁轻量的go语言框架，用于快速开发web应用

1. 实现context接口，用于传递请求上下文
2. 支持wild通配符路由匹配
3. 提供BaseController基类，简化控制器编写
4. 基于slog的日志库封装
    - 支持日志分级和自定义格式
    - 支持AddXXX方法
    - 使用context传递Field，可以在多个goroutine间使用
    - 支持日志轮转，可按文件大小、时间、行数自定义