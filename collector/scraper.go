package routerosreceiver

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"

	"github.com/sindrip/provider-routeros/rest"
	"github.com/sindrip/provider-routeros/routeros"

	"github.com/sindrip/provider-routeros/collector/internal/metadata"
)

type routerOSScraper struct {
	cfg      *Config
	settings receiver.Settings
	logger   *zap.Logger
	client   *rest.Client
	mb       *metadata.MetricsBuilder
}

func newRouterOSScraper(cfg *Config, params receiver.Settings) *routerOSScraper {
	return &routerOSScraper{
		cfg:      cfg,
		settings: params,
		logger:   params.Logger,
		mb:       metadata.NewMetricsBuilder(cfg.MetricsBuilderConfig, params),
	}
}

func (s *routerOSScraper) start(ctx context.Context, host component.Host) error {
	// confighttp owns TLS, proxying, auth extensions, compression and pooling.
	// rest takes the client rather than building a second policy beside it.
	c, err := s.cfg.ToClient(ctx, host.GetExtensions(), s.settings.TelemetrySettings)
	if err != nil {
		return err
	}
	s.client, err = rest.New(s.cfg.Endpoint,
		rest.WithHTTPClient(c),
		rest.WithBasicAuth(s.cfg.Username, string(s.cfg.Password)))
	return err
}

func (s *routerOSScraper) scrape(ctx context.Context) (pmetric.Metrics, error) {
	now := pcommon.NewTimestampFromTime(time.Now())

	if ifs, err := s.client.List(ctx, routeros.InterfacePath); err != nil {
		s.warn(routeros.InterfacePath, err)
	} else {
		for _, r := range ifs {
			v := routeros.DecodeInterface(r)
			s.mb.RecordRouterosInterfaceIoDataPoint(now, v.RxByte, v.Name, metadata.AttributeIoDirectionReceive)
			s.mb.RecordRouterosInterfaceIoDataPoint(now, v.TxByte, v.Name, metadata.AttributeIoDirectionTransmit)
			s.mb.RecordRouterosInterfaceUpDataPoint(now, gauge(v.Running), v.Name)
		}
	}

	if ss, err := s.client.List(ctx, routeros.RoutingBgpSessionPath); err != nil {
		s.warn(routeros.RoutingBgpSessionPath, err)
	} else {
		for _, r := range ss {
			v := routeros.DecodeRoutingBgpSession(r)
			s.mb.RecordRouterosBgpSessionPrefixCountDataPoint(now, v.PrefixCount, v.Name, v.RemoteAs)
			s.mb.RecordRouterosBgpSessionUpDataPoint(now, gauge(v.Established), v.Name, v.RemoteAs)
		}
	}

	// Generic sensor table: name is the id, type is the unit.
	if hs, err := s.client.List(ctx, routeros.SystemHealthPath); err != nil {
		s.warn(routeros.SystemHealthPath, err)
	} else {
		for _, r := range hs {
			v := routeros.DecodeSystemHealth(r)
			if v.Type != "C" {
				continue
			}
			// value is not a number on every row. The router states its
			// vocabulary as ok/fail/idle/no-input/not-present, so a sensor that
			// is unplugged reads "no-input" — and parsing that as a float, which
			// this receiver used to do unconditionally, reports 0 °C for a
			// sensor that said nothing at all. Record only what parses.
			celsius, ok := r.FloatOK("value")
			if !ok {
				s.logger.Debug("non-numeric health reading",
					zap.String("sensor", v.Name), zap.String("value", v.Value))
				continue
			}
			s.mb.RecordRouterosHwTemperatureDataPoint(now, celsius, v.Name)
		}
	}

	// Rules are keyed by comment, the same string as the CR external-name.
	if fs, err := s.client.List(ctx, routeros.IpFirewallFilterPath); err != nil {
		s.warn(routeros.IpFirewallFilterPath, err)
	} else {
		for _, r := range fs {
			v := routeros.DecodeIpFirewallFilter(r)
			if v.Comment == "" {
				continue
			}
			s.mb.RecordRouterosFirewallRuleIoDataPoint(now, v.Bytes, v.Comment)
		}
	}

	rb := s.mb.NewResourceBuilder()
	rb.SetServiceName("routeros")
	// Fall back to the endpoint so instance is never empty: without BOTH
	// service.name and service.instance.id the Prometheus exporter drops the
	// whole resource, taking model, serial and version with it — silently.
	instance := s.cfg.Endpoint
	if res, err := s.client.Get(ctx, routeros.SystemResourcePath); err != nil {
		s.warn(routeros.SystemResourcePath, err)
	} else {
		rb.SetRouterosOsVersion(routeros.DecodeSystemResource(res).Version)
	}
	// /system/routerboard is absent on some platforms — a CHR has no board —
	// and is absent from the generated types for the same reason, so it is read
	// as a raw record. A device without one is not an error worth surfacing.
	if board, err := s.client.Get(ctx, "/system/routerboard"); err == nil {
		rb.SetRouterosDeviceModel(board.String("model"))
		rb.SetRouterosDeviceSerial(board.String("serial-number"))
		if sn := board.String("serial-number"); sn != "" {
			instance = sn
		}
	}
	rb.SetServiceInstanceID(instance)
	s.mb.EmitForResource(metadata.WithResource(rb.Emit()))

	return s.mb.Emit(), nil
}

// gauge renders a boolean as the up/down series mdatagen expects; a gauge
// cannot hold a bool.
func gauge(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// warn logs a menu that did not answer, keeping the distinction rest draws.
// A device the router will not talk to at all is a different problem from a
// menu this hardware does not have, and one scrape failing should not read as
// the other.
func (s *routerOSScraper) warn(path string, err error) {
	var e *rest.Error
	switch {
	case errors.Is(err, rest.ErrAddressRejected):
		s.logger.Warn("router refused the source address; check /ip/service address lists",
			zap.String("menu", path))
	case errors.As(err, &e):
		s.logger.Warn("scrape failed", zap.String("menu", path),
			zap.Int("status", e.Status), zap.Int("code", e.Code), zap.String("detail", e.Detail))
	default:
		s.logger.Warn("scrape failed", zap.String("menu", path), zap.Error(err))
	}
}
