package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func authResponseBody() []byte {
	return []byte(fmt.Sprintf(`{"token": "access_token_here", "access_token": "access_token_here", "expires_in": 300, "issued_at": "%s" }`, time.Now().Format(time.RFC3339)))
}

type mockResponse struct {
	status   *int
	response []byte
	headers  http.Header
}

func subsequentRequestsFailHandler(firstResponse *mockResponse) http.HandlerFunc {
	requestCount := 0

	return func(w http.ResponseWriter, r *http.Request) {
		if requestCount == 0 {
			writeResponse(w, r, firstResponse)
			requestCount++
			return
		}

		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func basicAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)

		s := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
		if len(s) != 2 {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		b, err := base64.StdEncoding.DecodeString(s[1])
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		pair := strings.SplitN(string(b), ":", 2)
		if len(pair) != 2 {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		if pair[0] != "username" || pair[1] != "password" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		h.ServeHTTP(w, r)
	}
}

func handler(response *mockResponse) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeResponse(w, r, response)
	}
}

func writeResponse(w http.ResponseWriter, _ *http.Request, response *mockResponse) {
	if response.status != nil {
		w.WriteHeader(*response.status)
	}

	for h, values := range response.headers {
		for _, v := range values {
			w.Header().Add(h, v)
		}
	}

	_, _ = w.Write(response.response)
}

func expectMetrics(t *testing.T, c prometheus.Collector, fixture string) {
	exp, err := os.Open(path.Join("test", fixture))
	if err != nil {
		t.Fatalf("Error opening fixture file %q: %v", fixture, err)
	}
	if err := testutil.CollectAndCompare(c, exp); err != nil {
		t.Fatal("Unexpected metrics returned:", err)
	}
}

func TestHappyPath(t *testing.T) {
	authServer := httptest.NewServer(handler(&mockResponse{
		response: authResponseBody(),
	}))
	defer authServer.Close()

	rateLimitServer := httptest.NewServer(handler(&mockResponse{
		headers: map[string][]string{
			"RateLimit-Limit":     {"100;m21600"},
			"RateLimit-Remaining": {"76;m21600"},
		},
	}))
	defer rateLimitServer.Close()

	exporter := NewExporter(authServer.URL, rateLimitServer.URL, nil)
	expectMetrics(t, exporter, "success.metrics")
}

func TestHappyPathWithBasicAuth(t *testing.T) {
	authServer := httptest.NewServer(basicAuth(handler(&mockResponse{
		response: authResponseBody(),
	})))
	defer authServer.Close()

	rateLimitServer := httptest.NewServer(handler(&mockResponse{
		headers: map[string][]string{
			"RateLimit-Limit":     {"100;m21600"},
			"RateLimit-Remaining": {"76;m21600"},
		},
	}))
	defer rateLimitServer.Close()

	exporter := NewExporter(authServer.URL, rateLimitServer.URL,
		&credentials{
			username:   "username",
			passphrase: "password",
		})
	expectMetrics(t, exporter, "success.metrics")
}

func TestAuthTokenIsReusedWhenStillValid(t *testing.T) {
	authServer := httptest.NewServer(subsequentRequestsFailHandler(
		&mockResponse{
			response: authResponseBody(),
		}))
	defer authServer.Close()

	rateLimitServer := httptest.NewServer(handler(&mockResponse{
		headers: map[string][]string{
			"RateLimit-Limit":     {"100;m21600"},
			"RateLimit-Remaining": {"76;m21600"},
		},
	}))
	defer rateLimitServer.Close()

	exporter := NewExporter(authServer.URL, rateLimitServer.URL, nil)
	expectMetrics(t, exporter, "success.metrics")

	expectMetrics(t, exporter, "2nd-poll.metrics")
}

func TestUnableToAnonymouslyAuth(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	defer authServer.Close()

	rateLimitServer := httptest.NewServer(handler(&mockResponse{
		headers: map[string][]string{
			"RateLimit-Limit":     {"100;m21600"},
			"RateLimit-Remaining": {"76;m21600"},
		},
	}))
	defer rateLimitServer.Close()

	exporter := NewExporter(authServer.URL, rateLimitServer.URL, nil)
	expectMetrics(t, exporter, "failure.metrics")
}

func TestUnableToBasicAuth(t *testing.T) {
	authServer := httptest.NewServer(basicAuth(handler(&mockResponse{
		response: authResponseBody(),
	})))
	defer authServer.Close()

	rateLimitServer := httptest.NewServer(handler(&mockResponse{
		headers: map[string][]string{
			"RateLimit-Limit":     {"100;m21600"},
			"RateLimit-Remaining": {"76;m21600"},
		},
	}))
	defer rateLimitServer.Close()

	exporter := NewExporter(authServer.URL, rateLimitServer.URL,
		&credentials{
			username:   "username",
			passphrase: "not-the-correct-password",
		})
	expectMetrics(t, exporter, "failure.metrics")
}

func TestUnableToRetrieveRateLimit(t *testing.T) {
	authServer := httptest.NewServer(handler(&mockResponse{
		response: authResponseBody(),
	}))
	defer authServer.Close()

	rateLimitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer rateLimitServer.Close()

	exporter := NewExporter(authServer.URL, rateLimitServer.URL, nil)
	expectMetrics(t, exporter, "failure.metrics")
}

func TestMissingRateLimitHeadersIsTreatedAsAFailure(t *testing.T) {
	authServer := httptest.NewServer(handler(&mockResponse{
		response: authResponseBody(),
	}))
	defer authServer.Close()

	rateLimitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer rateLimitServer.Close()

	exporter := NewExporter(authServer.URL, rateLimitServer.URL, nil)
	expectMetrics(t, exporter, "failure.metrics")
}

func TestBadAuthURLFails(t *testing.T) {
	rateLimitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer rateLimitServer.Close()

	exporter := NewExporter("oh dear", rateLimitServer.URL, nil)
	expectMetrics(t, exporter, "failure.metrics")
}

func TestBadRateLimitServerURLFails(t *testing.T) {
	authServer := httptest.NewServer(handler(&mockResponse{
		response: authResponseBody(),
	}))
	defer authServer.Close()

	exporter := NewExporter(authServer.URL, "oh dear", nil)
	expectMetrics(t, exporter, "failure.metrics")
}

func TestBadJsonIsIgnored(t *testing.T) {
	authServer := httptest.NewServer(handler(&mockResponse{
		response: []byte("Whoops!"),
	}))
	defer authServer.Close()

	rateLimitServer := httptest.NewServer(handler(&mockResponse{
		headers: map[string][]string{
			"RateLimit-Limit":     {"100;m21600"},
			"RateLimit-Remaining": {"76;m21600"},
		},
	}))
	defer rateLimitServer.Close()

	exporter := NewExporter(authServer.URL, rateLimitServer.URL, nil)
	expectMetrics(t, exporter, "failure.metrics")
}

func TestTokenThatExpiresFarEnoughInTheFutureIsStillUsable(t *testing.T) {
	token := &AuthTokenResponse{
		ExpiresIn: tokenExpiryBufferInSeconds + 1,
		IssuedAt:  time.Now(),
	}

	if !token.isUsable(time.Now) {
		t.Fatalf("Auth Token should still be usable. %v", token.roughExpiry())
	}
}

func TestTokenThatExpiresRealSoonIsNotUsable(t *testing.T) {
	token := &AuthTokenResponse{
		ExpiresIn: tokenExpiryBufferInSeconds - 1,
		IssuedAt:  time.Now(),
	}

	if token.isUsable(time.Now) {
		t.Fatalf("Auth Token should still not be usable. %v", token.roughExpiry())
	}
}
