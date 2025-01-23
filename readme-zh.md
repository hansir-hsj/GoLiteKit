# golite

[英文](readme.md) [中文](readme-zh.md)

简洁轻量的go语言框架，用于快速开发web应用

1. 实现`context`接口，用于传递请求上下文
2. 支持wild通配符路由匹配
3. 提供`BaseController`基类，简化控制器编写
4. 基于`slog`的日志库封装
    - 支持日志分级和自定义格式
    - 支持AddXXX方法
    - 使用context传递Field，可以在多个goroutine间使用
    - 支持日志轮转，可按文件大小、时间、行数自定义
5. 支持中间件，下面是内部自带的一些中间件
   - 日志中间件
   - 超时中间件
   - 请求追踪中间件
   - 基于`golang.org/x/time/rate`的限流中间件