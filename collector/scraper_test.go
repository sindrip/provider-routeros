package routerosreceiver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver/receivertest"

	"github.com/sindrip/provider-routeros/collector/internal/metadata"
)

// menus are recorded RouterOS replies, kept as the router actually sends them:
// every value a string, booleans as "true"/"false", and /system/health's value
// column holding a state word on one row and a number on another.
var menus = map[string]string{
	"/rest/interface": `[
		{".id":"*1","name":"ether1","rx-byte":"17315240","tx-byte":"64547007","running":"true","disabled":"false"},
		{".id":"*2","name":"ether2","rx-byte":"144212","tx-byte":"354203","running":"false","disabled":"true"}
	]`,
	"/rest/routing/bgp/session": `[
		{".id":"*1","name":"peer-a","remote.as":"64512","prefix-count":"41","established":"true"},
		{".id":"*2","name":"peer-b","remote.as":"64513","prefix-count":"0","established":"false"}
	]`,
	"/rest/system/health": `[
		{".id":"*1","name":"cpu-temperature","type":"C","value":"47.5"},
		{".id":"*2","name":"board-temperature","type":"C","value":"no-input"},
		{".id":"*3","name":"psu1-voltage","type":"V","value":"12.1"}
	]`,
	"/rest/ip/firewall/filter": `[
		{".id":"*1","chain":"input","action":"accept","comment":"allow-ssh","bytes":"9001","packets":"42"},
		{".id":"*2","chain":"input","action":"drop","bytes":"7","packets":"1"}
	]`,
	"/rest/system/resource":    `{"version":"7.23.2 (stable)","uptime":"4h10m29s","cpu-load":"0"}`,
	"/rest/system/routerboard": `{"model":"CCR2004-1G-12S+2XS","serial-number":"HFF08X9RT2A"}`,
}

func newTestScraper(t *testing.T, srv *httptest.Server) *routerOSScraper {
	t.Helper()
	cfg := createDefaultConfig().(*Config)
	cfg.Endpoint = srv.URL
	cfg.Username = "admin"
	cfg.Password = "secret"

	s := newRouterOSScraper(cfg, receivertest.NewNopSettings(metadata.Type))
	if err := s.start(t.Context(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}
	return s
}

func serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "admin" || p != "secret" {
			// The router answers a bad credential with a JSON object whose
			// "error" is a *number*, which is why it cannot be decoded as an
			// ordinary record. rest sniffs it by shape.
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":401,"message":"Unauthorized","detail":"not logged in"}`))
			return
		}
		body, ok := menus[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":404,"message":"Not Found","detail":"no such command prefix"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// points flattens the emitted metrics to name -> attribute-set -> value, which
// is what the assertions actually care about.
func points(t *testing.T, m pmetric.Metrics) map[string]map[string]float64 {
	t.Helper()
	out := map[string]map[string]float64{}
	rms := m.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		sms := rms.At(i).ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			ms := sms.At(j).Metrics()
			for k := 0; k < ms.Len(); k++ {
				metric := ms.At(k)
				var dps pmetric.NumberDataPointSlice
				switch metric.Type() {
				case pmetric.MetricTypeGauge:
					dps = metric.Gauge().DataPoints()
				case pmetric.MetricTypeSum:
					dps = metric.Sum().DataPoints()
				default:
					continue
				}
				if out[metric.Name()] == nil {
					out[metric.Name()] = map[string]float64{}
				}
				for d := 0; d < dps.Len(); d++ {
					dp := dps.At(d)
					// Key on whatever attributes the point actually carries,
					// sorted, rather than on a guessed list -- guessing
					// collapsed every series onto one key and made the
					// assertions read the last row instead of the right one.
					attrs := dp.Attributes().AsRaw()
					names := make([]string, 0, len(attrs))
					for name := range attrs {
						names = append(names, name)
					}
					sort.Strings(names)
					var b strings.Builder
					for _, name := range names {
						if b.Len() > 0 {
							b.WriteByte(',')
						}
						fmt.Fprintf(&b, "%s=%v", name, attrs[name])
					}
					key := b.String()
					val := dp.DoubleValue()
					if dp.ValueType() == pmetric.NumberDataPointValueTypeInt {
						val = float64(dp.IntValue())
					}
					out[metric.Name()][key] = val
				}
			}
		}
	}
	return out
}

// TestScrapeDecodesRecordedReplies is the receiver's first behavioural test.
// It exists because the hand-rolled client it replaced had none, and because
// every value below was previously produced by a parser written three times in
// this codebase and never checked against a recorded reply.
func TestScrapeDecodesRecordedReplies(t *testing.T) {
	s := newTestScraper(t, serve(t))

	m, err := s.scrape(t.Context())
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	got := points(t, m)

	if len(got) == 0 {
		t.Fatal("no metrics emitted")
	}

	// Counters survive as exact integers rather than going through a float.
	assertPoint(t, got, "routeros.interface.io", 17315240)
	assertPoint(t, got, "routeros.bgp.session.prefix.count", 41)
	assertPoint(t, got, "routeros.firewall.rule.io", 9001)

	// "false" is false. The three RouterOS boolean encodings are the reason
	// this is worth asserting at all.
	upSeries := got["routeros.interface.up"]
	var ups, downs int
	for _, v := range upSeries {
		if v == 1 {
			ups++
		} else {
			downs++
		}
	}
	if ups != 1 || downs != 1 {
		t.Errorf("interface.up: %d up, %d down, want 1 and 1 (%v)", ups, downs, upSeries)
	}
}

// TestNonNumericSensorIsNotReportedAsZero pins the bug the migration fixed.
// The router answers an unplugged sensor with "no-input", and the previous
// implementation ran every value through a float parser, publishing 0 °C —
// a reading indistinguishable from a genuinely cold room.
func TestNonNumericSensorIsNotReportedAsZero(t *testing.T) {
	s := newTestScraper(t, serve(t))

	m, err := s.scrape(t.Context())
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	temps := points(t, m)["routeros.hw.temperature"]

	if len(temps) != 1 {
		t.Fatalf("want exactly one temperature series, got %d: %v", len(temps), temps)
	}
	for _, v := range temps {
		if v != 47.5 {
			t.Errorf("temperature = %v, want 47.5", v)
		}
	}
	// psu1-voltage is type V, not C, so it is not a temperature either.
	for key := range temps {
		if strings.Contains(key, "psu1-voltage") {
			t.Error("a voltage was published as a temperature")
		}
	}
}

// TestUnreachableMenuDoesNotFailTheScrape covers the platform difference that
// is normal rather than exceptional: a device without the menu still yields
// every other metric.
func TestUnreachableMenuDoesNotFailTheScrape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/system/routerboard" || r.URL.Path == "/rest/routing/bgp/session" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":404,"message":"Not Found","detail":"no such command prefix"}`))
			return
		}
		body, ok := menus[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":404,"message":"Not Found"}`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	cfg := createDefaultConfig().(*Config)
	cfg.Endpoint = srv.URL
	cfg.Username, cfg.Password = "admin", "secret"
	s := newRouterOSScraper(cfg, receivertest.NewNopSettings(metadata.Type))
	if err := s.start(t.Context(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}

	m, err := s.scrape(t.Context())
	if err != nil {
		t.Fatalf("a missing menu must not fail the scrape: %v", err)
	}
	got := points(t, m)
	if _, ok := got["routeros.interface.io"]; !ok {
		t.Error("interface metrics were lost because an unrelated menu was absent")
	}
}

func assertPoint(t *testing.T, got map[string]map[string]float64, metric string, want float64) {
	t.Helper()
	series, ok := got[metric]
	if !ok {
		t.Errorf("%s was not emitted", metric)
		return
	}
	for _, v := range series {
		if v == want {
			return
		}
	}
	t.Errorf("%s has no point equal to %v; got %v", metric, want, series)
}
