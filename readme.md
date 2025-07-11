# GoLiteKit

[en](readme.md) [zh](readme.zh.md)

A concise and lightweight Go language framework for rapid development of web applications.

1. Implement routing function based on 'go 1.22 http.ServeMux'
2. Implement the `context` interface to pass request context.
3. Provide a `BaseController` base class to simplify controller development.
4. Encapsulate a logging library based on official library `slog`:
    - Support log levels and custom log formats.
    - Support `AddXXX` methods.
    - Use `context` to pass `Field`, which can be used across multiple goroutines.
    - Support log rotation, customizable by file size, time, and line count.
5. Support middleware. Here are some built - in middleware:
    - Logging middleware
    - Timeout middleware
    - Request tracking middleware
    - Rate - limiting middleware based on `golang.org/x/time/rate`
6. Integrate gorm and go-redis framework
7. Provide a command-line tool *glk* to facilitate the quick creation of applications. Please use `go install github.com/hansir-hsj/GoLiteKit/glk@latest` to install it.