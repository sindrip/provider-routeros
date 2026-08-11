package connection

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sindrip/provider-routeros/rest"

	providerv1alpha1 "github.com/sindrip/provider-routeros/native/api/provider/v1alpha1"
)

const defaultRESTTimeout = 59 * time.Second

const minimumRequestTimeout = 5 * time.Second

type connectionSettings struct {
	Endpoint           string
	Username           string
	Password           string
	InsecureSkipVerify bool
	CABundle           []byte
	RequestTimeout     time.Duration
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
	Menu              Menu
	Fingerprint       string
	TargetFingerprint string
}

// ProviderConfigConnector resolves a namespaced ProviderConfig and builds a
// RouterOS REST client from Secret credentials in the same namespace.
type ProviderConfigConnector struct {
	mu      sync.Mutex
	clients map[string]cachedClient
}

type cachedClient struct {
	fingerprint [sha256.Size]byte
	menu        Menu
	http        *http.Client
}

// Connect returns a client for the named ProviderConfig in namespace.
func (c *ProviderConfigConnector) Connect(ctx context.Context, reader client.Reader, namespace, name string) (Connection, error) {
	if namespace == "" {
		return Connection{}, errors.New("ProviderConfig namespace is empty")
	}
	if name == "" {
		return Connection{}, errors.New("providerConfigRef.name is empty")
	}

	pc := &providerv1alpha1.ProviderConfig{}
	pcKey := types.NamespacedName{Namespace: namespace, Name: name}
	if err := reader.Get(ctx, pcKey, pc); err != nil {
		return Connection{}, fmt.Errorf("get ProviderConfig %s: %w", pcKey.String(), err)
	}
	settings, fingerprint, err := resolveSettings(ctx, reader, pc)
	if err != nil {
		return Connection{}, fmt.Errorf("ProviderConfig %s: %w", pcKey.String(), err)
	}
	fingerprintText := hex.EncodeToString(fingerprint[:])
	targetFingerprint := fingerprintTarget(settings.Endpoint)
	cacheKey := pcKey.String()
	c.mu.Lock()
	cached, ok := c.clients[cacheKey]
	c.mu.Unlock()
	if ok && cached.fingerprint == fingerprint {
		return Connection{Menu: cached.menu, Fingerprint: fingerprintText, TargetFingerprint: targetFingerprint}, nil
	}

	httpClient, err := newHTTPClient(settings)
	if err != nil {
		return Connection{}, fmt.Errorf("ProviderConfig %s: %w", pcKey.String(), err)
	}
	menu, err := rest.New(settings.Endpoint,
		rest.WithHTTPClient(httpClient),
		rest.WithBasicAuth(settings.Username, settings.Password),
	)
	if err != nil {
		httpClient.CloseIdleConnections()
		return Connection{}, err
	}
	c.mu.Lock()
	if c.clients == nil {
		c.clients = map[string]cachedClient{}
	}
	old := c.clients[cacheKey]
	if old.menu != nil && old.fingerprint == fingerprint {
		c.mu.Unlock()
		httpClient.CloseIdleConnections()
		return Connection{Menu: old.menu, Fingerprint: fingerprintText, TargetFingerprint: targetFingerprint}, nil
	}
	c.clients[cacheKey] = cachedClient{fingerprint: fingerprint, menu: menu, http: httpClient}
	c.mu.Unlock()
	if old.http != nil && old.http != httpClient {
		old.http.CloseIdleConnections()
	}
	return Connection{Menu: menu, Fingerprint: fingerprintText, TargetFingerprint: targetFingerprint}, nil
}

func resolveSettings(ctx context.Context, reader client.Reader, pc *providerv1alpha1.ProviderConfig) (connectionSettings, [sha256.Size]byte, error) {
	settings := connectionSettings{RequestTimeout: defaultRESTTimeout}
	endpoint, err := validateEndpoint(pc.Spec.Endpoint)
	if err != nil {
		return connectionSettings{}, [sha256.Size]byte{}, err
	}
	settings.Endpoint = endpoint
	if pc.Spec.TLS != nil && strings.HasPrefix(settings.Endpoint, "http://") {
		return connectionSettings{}, [sha256.Size]byte{}, errors.New("tls settings require an https endpoint")
	}
	if pc.Spec.RequestTimeout != nil {
		if pc.Spec.RequestTimeout.Duration < minimumRequestTimeout {
			return connectionSettings{}, [sha256.Size]byte{}, fmt.Errorf("requestTimeout must be at least %s", minimumRequestTimeout)
		}
		settings.RequestTimeout = pc.Spec.RequestTimeout.Duration
	}

	selector := pc.Spec.Credentials.SecretRef
	if selector.Name == "" {
		return connectionSettings{}, [sha256.Size]byte{}, errors.New("credentials.secretRef.name is required")
	}
	usernameKey := selector.UsernameKey
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := selector.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}
	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: selector.Name, Namespace: pc.Namespace}
	if err := reader.Get(ctx, key, secret); err != nil {
		return connectionSettings{}, [sha256.Size]byte{}, fmt.Errorf("get credentials Secret %s: %w", key.String(), err)
	}
	username, ok := secret.Data[usernameKey]
	if !ok {
		return connectionSettings{}, [sha256.Size]byte{}, fmt.Errorf("credentials Secret %s has no username key %q", key.String(), usernameKey)
	}
	password, ok := secret.Data[passwordKey]
	if !ok {
		return connectionSettings{}, [sha256.Size]byte{}, fmt.Errorf("credentials Secret %s has no password key %q", key.String(), passwordKey)
	}
	settings.Username = string(username)
	settings.Password = string(password)

	caName := ""
	caKey := ""
	if pc.Spec.TLS != nil {
		if pc.Spec.TLS.InsecureSkipVerify && pc.Spec.TLS.CASecretRef != nil {
			return connectionSettings{}, [sha256.Size]byte{}, errors.New("tls.insecureSkipVerify and tls.caSecretRef are mutually exclusive")
		}
		settings.InsecureSkipVerify = pc.Spec.TLS.InsecureSkipVerify
		if pc.Spec.TLS.CASecretRef != nil {
			caName = pc.Spec.TLS.CASecretRef.Name
			caKey = pc.Spec.TLS.CASecretRef.Key
			if caName == "" || caKey == "" {
				return connectionSettings{}, [sha256.Size]byte{}, errors.New("tls.caSecretRef.name and key are required")
			}
			caSecret := &corev1.Secret{}
			caSecretKey := types.NamespacedName{Name: caName, Namespace: pc.Namespace}
			if err := reader.Get(ctx, caSecretKey, caSecret); err != nil {
				return connectionSettings{}, [sha256.Size]byte{}, fmt.Errorf("get CA Secret %s: %w", caSecretKey.String(), err)
			}
			bundle, ok := caSecret.Data[caKey]
			if !ok {
				return connectionSettings{}, [sha256.Size]byte{}, fmt.Errorf("CA Secret %s has no key %q", caSecretKey.String(), caKey)
			}
			settings.CABundle = bundle
		}
	}

	fingerprint := fingerprintConnection(
		[]byte(pc.Namespace), []byte(pc.Name), []byte(settings.Endpoint),
		[]byte(selector.Name), []byte(usernameKey), username, []byte(passwordKey), password,
		[]byte(fmt.Sprintf("%t", settings.InsecureSkipVerify)), []byte(caName), []byte(caKey), settings.CABundle,
		[]byte(settings.RequestTimeout.String()),
	)
	return settings, fingerprint, nil
}

func fingerprintTarget(hostURL string) string {
	parsed, _ := url.Parse(hostURL)
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	value := sha256.Sum256([]byte(parsed.String()))
	return hex.EncodeToString(value[:])
}

func validateEndpoint(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", errors.New("endpoint is required")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("endpoint scheme %q is not REST-capable; use http or https", parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", errors.New("endpoint has no host")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func fingerprintConnection(parts ...[]byte) [sha256.Size]byte {
	hash := sha256.New()
	var size [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(size[:], uint64(len(part)))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write(part)
	}
	var fingerprint [sha256.Size]byte
	copy(fingerprint[:], hash.Sum(nil))
	return fingerprint
}

func newHTTPClient(settings connectionSettings) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if settings.InsecureSkipVerify {
		// This is explicit in ProviderConfig and needed for the self-signed
		// certificate RouterOS commonly ships with.
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
	}
	if len(settings.CABundle) > 0 {
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("load system certificate pool: %w", err)
		}
		if !pool.AppendCertsFromPEM(settings.CABundle) {
			return nil, errors.New("tls.caSecretRef contains no PEM certificates")
		}
		tlsConfig.RootCAs = pool
	}
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Transport: transport, Timeout: settings.RequestTimeout}, nil
}
