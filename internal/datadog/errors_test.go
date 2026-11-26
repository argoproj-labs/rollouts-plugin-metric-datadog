package datadog

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	datadogV1 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callMonitor runs one real SDK call against addr and returns the classified error.
func callMonitor(t *testing.T, addr string) error {
	t.Helper()
	creds := Credentials{APIKey: "AK", AppKey: "PK"}
	// A 1s timeout gives a 0.8s retry budget, shorter than the SDK's 2s first
	// backoff, so 429/5xx return without waiting on retries.
	client, err := NewClient(creds, ClientOptions{Address: addr, AllowInsecure: true, Timeout: time.Second})
	require.NoError(t, err)
	_, resp, err := datadogV1.NewMonitorsApi(client).GetMonitor(AuthContext(context.Background(), creds, ""), 1)
	require.Error(t, err)
	return MarkTransient(err, resp)
}

func TestMarkTransient_Statuses(t *testing.T) {
	cases := map[int]bool{
		400: false, 401: false, 403: false, 404: false,
		429: true, 500: true, 502: true, 503: true, 504: true,
	}
	for status, want := range cases {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"errors":["x"]}`))
		}))
		err := callMonitor(t, ts.URL)
		ts.Close()
		var te *TransientError
		assert.Equal(t, want, errors.As(err, &te), "status %d: %v", status, err)
	}
}

func TestMarkTransient_Timeout(t *testing.T) {
	done := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { <-done }))
	defer ts.Close()
	defer close(done)
	var te *TransientError
	assert.True(t, errors.As(callMonitor(t, ts.URL), &te))
}

func TestMarkTransient_ConnectionDropped(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		_ = conn.Close()
	}))
	defer ts.Close()
	var te *TransientError
	err := callMonitor(t, ts.URL)
	assert.True(t, errors.As(err, &te), err)
}

func TestMarkTransient_SetupErrorsFailClosed(t *testing.T) {
	// A closed port: connection refused, usually a mistyped address.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := "http://" + l.Addr().String()
	require.NoError(t, l.Close())

	for _, a := range []string{addr, "http://no-such-host.invalid"} {
		err := callMonitor(t, a)
		var te *TransientError
		assert.False(t, errors.As(err, &te), "%s: %v", a, err)
	}
}

func TestMarkTransient_KeepsMessage(t *testing.T) {
	inner := errors.New("datadog GetMonitor: 503 Service Unavailable")
	err := MarkTransient(inner, &http.Response{StatusCode: 503})
	assert.Equal(t, inner.Error(), err.Error())
	assert.ErrorIs(t, err, inner)
	assert.Nil(t, MarkTransient(nil, nil))
}
