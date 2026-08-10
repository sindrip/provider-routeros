package routerosreceiver

import (
	"errors"

	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/scraper/scraperhelper"

	"github.com/sindrip/provider-routeros/collector/internal/metadata"
)

// Config is the receiver's configuration. Everything device-specific is
// config, not code — which is what lets one collector serve a fleet.
type Config struct {
	scraperhelper.ControllerConfig `mapstructure:",squash"`
	confighttp.ClientConfig        `mapstructure:",squash"`

	Username string              `mapstructure:"username"`
	Password configopaque.String `mapstructure:"password"`

	metadata.MetricsBuilderConfig `mapstructure:",squash"`
}

func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("endpoint is required (e.g. https://10.0.99.1)")
	}
	if c.Username == "" {
		return errors.New("username is required")
	}
	return nil
}
