package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
)

const (
	namespace                  = "dockerhub" // For Prometheus metric
	tokenExpiryBufferInSeconds = 2           // the amount of NTP drift we tolerate when considering whether a token might have expired
)

// Exporter collects Docker Hub rate limit stats and exports them using the prometheus
// metrics package.
type Exporter struct {
	mu sync.RWMutex

	authServerURL string
	rateLimitURL  string
	credentials   *credentials

	clock func() time.Time

	totalScrapes, scrapeFailures prometheus.Counter
	remaining, limit             prometheus.Gauge
	authToken                    *AuthTokenResponse
}

// NewExporter returns an initialized Exporter.
func NewExporter(authServerURL string, rateLimitURL string, credentials *credentials) *Exporter {
	return &Exporter{

		authServerURL: authServerURL,
		rateLimitURL:  rateLimitURL,
		credentials:   credentials,

		clock: time.Now,
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_scrapes_total",
			Help:      "Current total Docker Hub scrapes.",
		}),
		scrapeFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "exporter_poll_failures_total",
			Help:      "Number of errors while polling Docker Hub.",
		}),
		remaining: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "limit_remaining_requests_total",
			Help:      "Docker Hub Rate Limit Remaining Requests",
		}),
		limit: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "limit_max_requests_total",
			Help:      "Docker Hub Rate Limit Maximum Requests",
		}),
	}
}

// Collect fetches the stats from configured Docker Hub location and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mu.Lock() // To protect metrics from concurrent collects.
	defer e.mu.Unlock()

	e.scrape(ch)

	ch <- e.limit
	ch <- e.remaining

	ch <- e.totalScrapes
	ch <- e.scrapeFailures
}

// Describe describes all the metrics ever exported by the Docker Hub exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.limit.Desc()
	ch <- e.remaining.Desc()

	ch <- e.totalScrapes.Desc()
	ch <- e.scrapeFailures.Desc()
}

func (e *Exporter) scrape(ch chan<- prometheus.Metric) {
	e.totalScrapes.Inc()

	rateLimit, remaining, err := e.fetchRateLimit()

	if err != nil {
		fmt.Printf("%+v\n", err)
		e.scrapeFailures.Inc()
		return
	}

	e.limit.Set(rateLimit)
	e.remaining.Set(remaining)
}

func (e *Exporter) fetchRateLimit() (limit float64, remaining float64, err error) {
	token, err := e.fetchToken()

	if err != nil {
		return
	}

	req, err := http.NewRequest("HEAD", e.rateLimitURL, nil)
	if err != nil {
		return 0, 0, err
	}

	req.Header.Set("Authorization", "Bearer "+*token)
	res, err := fetchHTTP(req)

	if err != nil {
		return 0, 0, err
	}

	defer res.Body.Close()

	limit, err = parseFloat(res.Header.Get("RateLimit-Limit"))

	if err != nil {
		return
	}

	remaining, err = parseFloat(res.Header.Get("RateLimit-Remaining"))

	return
}

// parseFloat takes the header value 76;w=21600 (76 per 6 hours) and extracts the first part
func parseFloat(s string) (float64, error) {
	value := strings.Split(strings.TrimSpace(s), ";")[0]
	return strconv.ParseFloat(value, 64)
}

// AuthTokenResponse is used for parsing the JSON response coming back from Docker Hub
type AuthTokenResponse struct {
	Token       string    `json:"token"`
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	IssuedAt    time.Time `json:"issued_at"`
}

func (a *AuthTokenResponse) isUsable(now func() time.Time) bool {
	return now().Before(a.roughExpiry())
}

// roughExpiry returns the expiry time of this token, minus a bit. The expiry time is calculated
// from when this token was issued, plus the duration that it's valid for. We minus a bit to allow
// for some clock drift (which nobody has in production, amirite?) and also to ensure we don't try
// re-use a token just before it expires.
func (a *AuthTokenResponse) roughExpiry() time.Time {
	// Internally, we consider it `tokenExpiryBufferInSeconds` seconds earlier than the actual
	// expiry. This number is entirely random. If your NTP service is more than
	// `tokenExpiryBufferInSeconds` seconds out, you should fix that.
	return a.IssuedAt.Add(time.Second * time.Duration(a.ExpiresIn-tokenExpiryBufferInSeconds))
}

func (e *Exporter) hasUsableToken() bool {
	if e.authToken == nil {
		return false
	}

	return e.authToken.isUsable(e.clock)
}

func (e *Exporter) fetchToken() (*string, error) {
	if e.hasUsableToken() {
		return &e.authToken.AccessToken, nil
	}

	req, err := http.NewRequest("GET", e.authServerURL, nil)

	if err != nil {
		return nil, err
	}

	if e.credentials != nil {
		req.SetBasicAuth(e.credentials.username, e.credentials.passphrase)
	}

	r, err := fetchHTTP(req)

	if err != nil {
		return nil, err
	}

	defer r.Body.Close()

	var token AuthTokenResponse

	dec := json.NewDecoder(r.Body)

	if err = dec.Decode(&token); err != nil {
		return nil, err
	}

	e.authToken = &token

	return &token.Token, nil
}

func fetchHTTP(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return nil, err
	}

	if !(resp.StatusCode >= 200 && resp.StatusCode < 300) {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	return resp, nil
}

type arguments struct {
	credentials *credentials
	port        string
	metricsPath string
}

type credentials struct {
	username, passphrase string
}

func main() {
	args := parseAndVerifyArgs()

	exporter := NewExporter("https://auth.docker.io/token?service=registry.docker.io&scope=repository:ratelimitpreview/test:pull", "https://registry-1.docker.io/v2/ratelimitpreview/test/manifests/latest", args.credentials)
	prometheus.MustRegister(exporter)
	prometheus.MustRegister(version.NewCollector("dockerhub_exporter"))

	http.DefaultClient.Timeout = time.Second * 5

	http.Handle(args.metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Docker Hub Exporter</title></head>
             <body>
             <h1>Docker Hub Exporter</h1>
             <p><a href='` + args.metricsPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})

	if err := http.ListenAndServe(":"+args.port, nil); err != nil {
		fmt.Printf("Error starting HTTP server: %v", err)
		os.Exit(1)
	}
}

func parseAndVerifyArgs() *arguments {
	var (
		help    bool
		version bool

		username   string
		passphrase string
	)

	res := &arguments{}
	flag.StringVar(&res.port, "port", "9090", "Port to listen on")
	flag.StringVar(&res.metricsPath, "path", "/metrics", "Path to expose metrics on")
	flag.StringVar(&username, "user", "", "Optional username to authenticate with")
	flag.StringVar(&passphrase, "pass", "", "Optional passphrase to authenticate with")
	flag.BoolVar(&version, "version", false, "Display version and exit")
	flag.BoolVar(&help, "h", false, "Display this help message")
	flag.BoolVar(&help, "help", false, "Display this help message")

	flag.Usage = func() {
		basename := filepath.Base(os.Args[0])
		fmt.Printf("Usage: %s\n", basename)
		flag.PrintDefaults()
	}

	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(1)
	}

	if version {
		fmt.Printf("%s\n", os.Args[0])
		os.Exit(1)
	}

	if res.port == "" {
		flag.Usage()
		os.Exit(2)
	}

	if username != "" && passphrase != "" {
		res.credentials = &credentials{username: username, passphrase: passphrase}
	}

	return res
}
