package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/argoproj-labs/rollouts-plugin-metric-datadog/internal/config"
	ddinternal "github.com/argoproj-labs/rollouts-plugin-metric-datadog/internal/datadog"
	"github.com/argoproj-labs/rollouts-plugin-metric-datadog/internal/datasource"
	"github.com/argoproj/argo-rollouts/pkg/apis/rollouts/v1alpha1"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type errSource struct{ err error }

func (e errSource) Query(_ context.Context, _ *datadog.APIClient, _ *config.Config) (datasource.Result, error) {
	return datasource.Result{}, e.err
}
func (e errSource) Key(_ *config.Config) string { return "err" }

func TestRun_OnError(t *testing.T) {
	transient := &ddinternal.TransientError{Err: errors.New("datadog GetMonitor: 503 Service Unavailable")}
	setup := errors.New("datadog GetMonitor: 403 Forbidden")
	cases := []struct {
		onError string
		err     error
		want    v1alpha1.AnalysisPhase
		class   string
	}{
		{"", transient, v1alpha1.AnalysisPhaseError, "datadog"},
		{"error", transient, v1alpha1.AnalysisPhaseError, "datadog"},
		{"inconclusive", transient, v1alpha1.AnalysisPhaseInconclusive, "datadog"},
		{"pass", transient, v1alpha1.AnalysisPhaseSuccessful, "datadog"},
		{"", setup, v1alpha1.AnalysisPhaseError, ""},
		{"inconclusive", setup, v1alpha1.AnalysisPhaseError, ""},
		{"pass", setup, v1alpha1.AnalysisPhaseError, ""},
	}
	for _, tc := range cases {
		name := fmt.Sprintf("%q/%v", tc.onError, tc.err)
		g := newTestPlugin(t, errSource{err: tc.err})
		raw := `{"monitor":{"id":1}}`
		if tc.onError != "" {
			raw = fmt.Sprintf(`{"onError":%q,"monitor":{"id":1}}`, tc.onError)
		}
		out := g.Run(&v1alpha1.AnalysisRun{}, metricWith(t, raw))
		assert.Equal(t, tc.want, out.Phase, name)
		assert.Equal(t, tc.err.Error(), out.Message, name)
		assert.Equal(t, tc.class, out.Metadata["error_class"], name)
		assert.NotNil(t, out.FinishedAt, name)
	}
}

func TestRun_OnErrorAgainstRealServer(t *testing.T) {
	t.Setenv("DD_API_KEY", "")
	t.Setenv("DD_APP_KEY", "")
	for status, want := range map[int]v1alpha1.AnalysisPhase{
		503: v1alpha1.AnalysisPhaseSuccessful,
		403: v1alpha1.AnalysisPhaseError,
	} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		g := &RpcPlugin{
			LogCtx:       *log.WithField("test", t.Name()),
			resolver:     &ddinternal.Resolver{Secrets: stubSecrets{}, ControllerNamespace: "argo-rollouts"},
			selectSource: datasource.Select,
			newClient: func(creds ddinternal.Credentials, opts ddinternal.ClientOptions) (*datadog.APIClient, error) {
				opts.Address = ts.URL
				opts.AllowInsecure = true
				return ddinternal.NewClient(creds, opts)
			},
		}
		// timeoutSeconds 1 keeps the SDK retry budget below its first 2s backoff.
		m := metricWith(t, `{"onError":"pass","timeoutSeconds":1,"monitor":{"id":1}}`)
		out := g.Run(&v1alpha1.AnalysisRun{}, m)
		ts.Close()
		assert.Equal(t, want, out.Phase, "status %d: %s", status, out.Message)
		assert.Contains(t, out.Message, fmt.Sprint(status))
	}
}
