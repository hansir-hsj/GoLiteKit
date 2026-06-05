package otel

import (
	glk "github.com/hansir-hsj/GoLiteKit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func mapAttributes(attrs []glk.Attribute) []attribute.KeyValue {
	mapped := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		switch value := attr.Value.AsInterface().(type) {
		case string:
			mapped = append(mapped, attribute.String(attr.Key, value))
		case int:
			mapped = append(mapped, attribute.Int(attr.Key, value))
		case bool:
			mapped = append(mapped, attribute.Bool(attr.Key, value))
		case float64:
			mapped = append(mapped, attribute.Float64(attr.Key, value))
		default:
			mapped = append(mapped, attribute.String(attr.Key, "<unsupported>"))
		}
	}
	return mapped
}

func mapStatus(status glk.SpanStatus) codes.Code {
	switch status {
	case glk.SpanStatusOK:
		return codes.Ok
	case glk.SpanStatusError:
		return codes.Error
	default:
		return codes.Unset
	}
}
