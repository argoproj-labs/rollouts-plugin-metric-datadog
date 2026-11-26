package datasource

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/argoproj-labs/rollouts-plugin-metric-datadog/internal/config"
	ddinternal "github.com/argoproj-labs/rollouts-plugin-metric-datadog/internal/datadog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every SDK call site must mark 5xx as transient and leave 4xx unmarked.
func TestSources_MarkTransientErrors(t *testing.T) {
	id := int64(1)
	sloID := "abc"
	sources := map[string]*config.Config{
		"metrics v2":     {Metrics: &config.MetricsConfig{APIVersion: "v2", Query: "avg:cpu{*}"}},
		"metrics v1":     {Metrics: &config.MetricsConfig{APIVersion: "v1", Query: "avg:cpu{*}"}},
		"monitor id":     {Monitor: &config.MonitorConfig{ID: &id}},
		"monitor search": {Monitor: &config.MonitorConfig{Query: "service:x"}},
		"slo id":         {SLO: &config.SLOConfig{ID: &sloID}},
		"slo search":     {SLO: &config.SLOConfig{Query: "service:x"}},
	}
	for name, cfg := range sources {
		for status, want := range map[int]bool{503: true, 403: false} {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			creds := ddinternal.Credentials{APIKey: "AK", AppKey: "PK"}
			// 1s timeout keeps the SDK retry budget below its first 2s backoff.
			client, err := ddinternal.NewClient(creds, ddinternal.ClientOptions{Address: ts.URL, AllowInsecure: true, Timeout: time.Second})
			require.NoError(t, err)
			ds, err := Select(cfg)
			require.NoError(t, err)
			_, err = ds.Query(ddinternal.AuthContext(context.Background(), creds, ""), client, cfg)
			ts.Close()
			require.Error(t, err, name)
			var te *ddinternal.TransientError
			assert.Equal(t, want, errors.As(err, &te), "%s %d: %v", name, status, err)
		}
	}
}
