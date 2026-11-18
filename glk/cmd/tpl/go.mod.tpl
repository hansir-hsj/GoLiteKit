module {{.Module}}

go 1.23
{{- if .FrameworkVersion}}

require github.com/hansir-hsj/GoLiteKit {{.FrameworkVersion}}
{{- end}}
