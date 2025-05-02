package traces

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestNewTracer(t *testing.T) {
	tests := []struct {
		name         string
		contextSetup func() (context.Context, func())
		wantProvider bool
		wantCleanup  bool
		wantErr      bool
	}{
		{
			name: "successful initialization",
			contextSetup: func() (context.Context, func()) {
				return context.WithTimeout(context.Background(), 5*time.Second)
			},
			wantProvider: true,
			wantCleanup:  true,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := tt.contextSetup()
			defer cancel()

			provider, cleanup, err := NewTracer(ctx)

			if tt.wantProvider {
				assert.NotNil(t, provider)
				assert.IsType(t, &sdktrace.TracerProvider{}, provider)
				assert.Equal(t, provider, otel.GetTracerProvider())
			} else {
				assert.Nil(t, provider)
			}

			if tt.wantCleanup {
				assert.NotNil(t, cleanup)
				cleanup()
			} else {
				assert.Nil(t, cleanup)
			}

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
