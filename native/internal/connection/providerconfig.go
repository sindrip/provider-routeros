package connection

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1beta1 "github.com/sindrip/provider-routeros/apis/cluster/v1beta1"
	"github.com/sindrip/provider-routeros/rest"
)

const defaultRESTTimeout = 59 * time.Second

// Credentials is the REST-relevant subset of the existing Terraform provider
// credential document. Keeping the same JSON shape lets one ProviderConfig be
// shared by the shipping and native controllers.
type Credentials struct {
	HostURL       string `json:"hosturl"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Insecure      bool   `json:"insecure"`
	CACertificate string `json:"ca_certificate"`
	RESTTimeout   int    `json:"rest_timeout"`
}

// Menu is the part of the REST client used by a menu controller.
type Menu interface {
	Plan(context.Context, rest.MenuSpec, []rest.Record) (rest.Plan, error)
	Apply(context.Context, rest.MenuSpec, []rest.Record) (rest.Plan, error)
	ApplyChecked(context.Context, rest.MenuSpec, []rest.Record, func(rest.Plan) error) (rest.Plan, error)
}

// Connection binds a REST client to a fingerprint of the ProviderConfig and
// Secret material that selected its router. Destructive adoption approval is
// tied to this value, so repointing credentials requires a fresh preview.
type Connection struct {
	Menu        Menu
	Fingerprint string
}

// ProviderConfigConnector resolves the existing cluster ProviderConfig and
// builds a RouterOS REST client from its Secret credentials.
type ProviderConfigConnector struct {
	mu      sync.Mutex
	clients map[string]cachedClient
}

type cachedClient struct {
	fingerprint [sha256.Size]byte
	menu        Menu
	http        *http.Client
}

// Connect returns a client for the named ProviderConfig.
func (c *ProviderConfigConnector) Connect(ctx context.Context, reader client.Reader, name string) (Connection, error) {
	if name == "" {
		return Connection{}, errors.New("providerConfigRef.name is empty")
	}

	pc := &clusterv1beta1.ProviderConfig{}
	if err := reader.Get(ctx, types.NamespacedName{Name: name}, pc); err != nil {
		return Connection{}, fmt.Errorf("get ProviderConfig %q: %w", name, err)
	}
	if pc.Spec.Credentials.Source != xpv2.CredentialsSourceSecret {
		return Connection{}, fmt.Errorf("ProviderConfig %q uses credentials source %q; native REST currently supports Secret", name, pc.Spec.Credentials.Source)
	}
	selector := pc.Spec.Credentials.SecretRef
	if selector == nil {
		return Connection{}, fmt.Errorf("ProviderConfig %q has no credentials.secretRef", name)
	}

	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: selector.Name, Namespace: selector.Namespace}
	if err := reader.Get(ctx, key, secret); err != nil {
		return Connection{}, fmt.Errorf("get ProviderConfig %q credentials Secret %s: %w", name, key.String(), err)
	}
	raw, ok := secret.Data[selector.Key]
	if !ok {
		return Connection{}, fmt.Errorf("ProviderConfig %q credentials Secret %s has no key %q", name, key.String(), selector.Key)
	}

	credentials, err := parseCredentials(raw)
	if err != nil {
		return Connection{}, fmt.Errorf("ProviderConfig %q credentials: %w", name, err)
	}
	connectionMaterial := []byte(name + "\x00" + selector.Namespace + "\x00" + selector.Name + "\x00" + selector.Key + "\x00")
	fingerprint := sha256.Sum256(append(connectionMaterial, raw...))
	fingerprintText := hex.EncodeToString(fingerprint[:])
	if credentials.CACertificate == "" {
		c.mu.Lock()
		cached, ok := c.clients[name]
		c.mu.Unlock()
		if ok && cached.fingerprint == fingerprint {
			return Connection{Menu: cached.menu, Fingerprint: fingerprintText}, nil
		}
	}

	httpClient, err := newHTTPClient(credentials)
	if err != nil {
		return Connection{}, fmt.Errorf("ProviderConfig %q credentials: %w", name, err)
	}
	menu, err := rest.New(credentials.HostURL,
		rest.WithHTTPClient(httpClient),
		rest.WithBasicAuth(credentials.Username, credentials.Password),
	)
	if err != nil {
		return Connection{}, err
	}
	// A CA file may change independently of the Kubernetes objects that name
	// it, so those clients are intentionally rebuilt on every poll.
	if credentials.CACertificate == "" {
		c.mu.Lock()
		if c.clients == nil {
			c.clients = map[string]cachedClient{}
		}
		old := c.clients[name]
		if old.menu != nil && old.fingerprint == fingerprint {
			c.mu.Unlock()
			httpClient.CloseIdleConnections()
			return Connection{Menu: old.menu, Fingerprint: fingerprintText}, nil
		}
		c.clients[name] = cachedClient{fingerprint: fingerprint, menu: menu, http: httpClient}
		c.mu.Unlock()
		if old.http != nil && old.http != httpClient {
			old.http.CloseIdleConnections()
		}
	}
	return Connection{Menu: menu, Fingerprint: fingerprintText}, nil
}

func parseCredentials(raw []byte) (Credentials, error) {
	var credentials Credentials
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return Credentials{}, fmt.Errorf("decode JSON: %w", err)
	}
	credentials.HostURL = strings.TrimSpace(credentials.HostURL)
	if credentials.HostURL == "" {
		return Credentials{}, errors.New("hosturl is required")
	}
	if !strings.Contains(credentials.HostURL, "://") {
		credentials.HostURL = "https://" + credentials.HostURL
	}
	parsed, err := url.Parse(credentials.HostURL)
	if err != nil {
		return Credentials{}, fmt.Errorf("parse hosturl: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Credentials{}, fmt.Errorf("hosturl scheme %q is not REST-capable; use http or https", parsed.Scheme)
	}
	if parsed.Host == "" {
		return Credentials{}, errors.New("hosturl has no host")
	}
	if credentials.Insecure && credentials.CACertificate != "" {
		return Credentials{}, errors.New("insecure and ca_certificate are mutually exclusive")
	}
	if credentials.RESTTimeout < 0 {
		return Credentials{}, errors.New("rest_timeout cannot be negative")
	}
	if credentials.RESTTimeout > 0 && credentials.RESTTimeout < 5 {
		return Credentials{}, errors.New("rest_timeout must be at least 5 seconds when set")
	}
	return credentials, nil
}

func newHTTPClient(credentials Credentials) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if credentials.Insecure {
		// This is an explicit option in the shared credentials document and is
		// needed for the self-signed certificate RouterOS commonly ships with.
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
	}
	if credentials.CACertificate != "" {
		pem, err := os.ReadFile(credentials.CACertificate)
		if err != nil {
			return nil, fmt.Errorf("read ca_certificate %q: %w", credentials.CACertificate, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("load system certificate pool: %w", err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_certificate %q contains no PEM certificates", credentials.CACertificate)
		}
		tlsConfig.RootCAs = pool
	}
	transport.TLSClientConfig = tlsConfig

	timeout := defaultRESTTimeout
	if credentials.RESTTimeout > 0 {
		timeout = time.Duration(credentials.RESTTimeout) * time.Second
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}
