package routerosreceiver

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/scraper"
	"go.opentelemetry.io/collector/scraper/scraperhelper"

	"github.com/sindrip/provider-routeros/collector/internal/metadata"
)

func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		metadata.Type,
		createDefaultConfig,
		receiver.WithMetrics(createMetrics, metadata.MetricsStability),
	)
}

func createDefaultConfig() component.Config {
	cc := scraperhelper.NewDefaultControllerConfig()
	cc.CollectionInterval = 30 * time.Second

	hc := confighttp.NewDefaultClientConfig()
	hc.Timeout = 10 * time.Second

	return &Config{
		ControllerConfig:     cc,
		ClientConfig:         hc,
		Username:             "admin",
		MetricsBuilderConfig: metadata.DefaultMetricsBuilderConfig(),
	}
}

func createMetrics(
	_ context.Context,
	params receiver.Settings,
	rConf component.Config,
	next consumer.Metrics,
) (receiver.Metrics, error) {
	cfg := rConf.(*Config)

	s := newRouterOSScraper(cfg, params)
	sc, err := scraper.NewMetrics(s.scrape, scraper.WithStart(s.start))
	if err != nil {
		return nil, err
	}

	return scraperhelper.NewMetricsController(
		&cfg.ControllerConfig, params, next,
		scraperhelper.AddScraper(metadata.Type, sc),
	)
}
