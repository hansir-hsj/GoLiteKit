package otel

import (
	"testing"

	glk "github.com/hansir-hsj/GoLiteKit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func TestMapAttributes(t *testing.T) {
	attrs := []glk.Attribute{
		glk.StringAttr("string", "value"),
		glk.IntAttr("int", 42),
		glk.BoolAttr("bool", true),
		glk.FloatAttr("float", 3.14),
		{Key: "unsupported"},
	}

	got := mapAttributes(attrs)
	want := []attribute.KeyValue{
		attribute.String("string", "value"),
		attribute.Int("int", 42),
		attribute.Bool("bool", true),
		attribute.Float64("float", 3.14),
		attribute.String("unsupported", "<unsupported>"),
	}

	if len(got) != len(want) {
		t.Fatalf("mapAttributes returned %d attributes, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("attribute %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSpanStatusMapping(t *testing.T) {
	tests := []struct {
		name   string
		status glk.SpanStatus
		want   codes.Code
	}{
		{name: "unset", status: glk.SpanStatusUnset, want: codes.Unset},
		{name: "ok", status: glk.SpanStatusOK, want: codes.Ok},
		{name: "error", status: glk.SpanStatusError, want: codes.Error},
		{name: "unknown", status: glk.SpanStatus(99), want: codes.Unset},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapStatus(tt.status); got != tt.want {
				t.Fatalf("mapStatus(%v) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestOptionsCloneMetricAttributeLabels(t *testing.T) {
	labels := []string{"route", "method"}
	options := applyOptions([]Option{WithMetricAttributeLabels(labels...)})

	labels[0] = "mutated"
	if options.MetricAttributeLabels[0] != "route" {
		t.Fatalf("MetricAttributeLabels changed after caller mutation: %v", options.MetricAttributeLabels)
	}

}
