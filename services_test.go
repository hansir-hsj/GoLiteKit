package golitekit

import "testing"

func TestWithServiceRejectsInvalidRegistration(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value any
	}{
		{name: "empty key", key: "", value: struct{}{}},
		{name: "nil value", key: "cache", value: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()

			WithService(tt.key, tt.value)(&Services{})
		})
	}
}
