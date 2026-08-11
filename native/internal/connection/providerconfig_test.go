package connection

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	providerv1alpha1 "github.com/sindrip/provider-routeros/native/api/provider/v1alpha1"
)

func TestValidateEndpoint(t *testing.T) {
	tests := map[string]struct {
		endpoint string
		want     string
		wantErr  bool
	}{
		"https":           {endpoint: "https://router.example/", want: "https://router.example"},
		"http":            {endpoint: "http://router.example/rest", want: "http://router.example/rest"},
		"requires scheme": {endpoint: "router.example", wantErr: true},
		"rejects API":     {endpoint: "api://router.example", wantErr: true},
		"requires host":   {endpoint: "https://", wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := validateEndpoint(test.endpoint)
			if test.wantErr {
				if err == nil {
					t.Fatalf("validateEndpoint() succeeded with %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateEndpoint() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("endpoint = %q, want %q", got, test.want)
			}
		})
	}
}

func TestProviderConfigConnectorUsesLocalSecretKeys(t *testing.T) {
	scheme := newTestScheme(t)
	pc := newProviderConfig("tenant-a", "https://router.example")
	pc.Spec.TLS = &providerv1alpha1.ProviderTLSConfig{InsecureSkipVerify: true}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "router-creds", Namespace: "tenant-a"},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pc, secret).Build()

	connector := &ProviderConfigConnector{}
	connected, err := connector.Connect(context.Background(), reader, "tenant-a", "router")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if connected.Menu == nil || len(connected.Fingerprint) != 64 || len(connected.TargetFingerprint) != 64 {
		t.Fatalf("Connect() returned an incomplete connection: %#v", connected)
	}
	again, err := connector.Connect(context.Background(), reader, "tenant-a", "router")
	if err != nil {
		t.Fatalf("second Connect() error = %v", err)
	}
	if connected.Menu != again.Menu || connected.Fingerprint != again.Fingerprint {
		t.Fatal("unchanged ProviderConfig and Secret did not reuse their REST client")
	}

	updated := &corev1.Secret{}
	key := client.ObjectKey{Name: "router-creds", Namespace: "tenant-a"}
	if err := reader.Get(context.Background(), key, updated); err != nil {
		t.Fatal(err)
	}
	updated.Data["password"] = []byte("rotated")
	if err := reader.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	rotated, err := connector.Connect(context.Background(), reader, "tenant-a", "router")
	if err != nil {
		t.Fatalf("Connect() after credential update error = %v", err)
	}
	if rotated.Fingerprint == connected.Fingerprint || rotated.Menu == connected.Menu {
		t.Fatal("changed credentials reused their adoption identity or REST client")
	}
	if rotated.TargetFingerprint != connected.TargetFingerprint {
		t.Fatal("credential rotation changed the RouterOS target identity")
	}
}

func TestProviderConfigConnectorUsesCustomKeys(t *testing.T) {
	scheme := newTestScheme(t)
	pc := newProviderConfig("tenant-a", "http://router.example")
	pc.Spec.Credentials.SecretRef.UsernameKey = "user"
	pc.Spec.Credentials.SecretRef.PasswordKey = "pass"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "router-creds", Namespace: "tenant-a"},
		Data:       map[string][]byte{"user": []byte("admin"), "pass": []byte("secret")},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pc, secret).Build()
	if _, err := (&ProviderConfigConnector{}).Connect(context.Background(), reader, "tenant-a", "router"); err != nil {
		t.Fatalf("Connect() with custom Secret keys error = %v", err)
	}
}

func TestProviderConfigConnectorReadsCASecretAndInvalidatesCache(t *testing.T) {
	certificate := selfSignedCertificate(t)

	scheme := newTestScheme(t)
	pc := newProviderConfig("tenant-a", "https://router.example")
	pc.Spec.TLS = &providerv1alpha1.ProviderTLSConfig{CASecretRef: &xpv2.LocalSecretKeySelector{Name: "router-ca", Key: "ca.crt"}}
	credentials := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "router-creds", Namespace: "tenant-a"},
		Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("secret")},
	}
	ca := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "router-ca", Namespace: "tenant-a"},
		Data:       map[string][]byte{"ca.crt": certificate},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pc, credentials, ca).Build()
	connector := &ProviderConfigConnector{}
	connected, err := connector.Connect(context.Background(), reader, "tenant-a", "router")
	if err != nil {
		t.Fatalf("Connect() with CA Secret error = %v", err)
	}

	updated := &corev1.Secret{}
	if err := reader.Get(context.Background(), client.ObjectKeyFromObject(ca), updated); err != nil {
		t.Fatal(err)
	}
	updated.Data["ca.crt"] = append(updated.Data["ca.crt"], '\n')
	if err := reader.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	changed, err := connector.Connect(context.Background(), reader, "tenant-a", "router")
	if err != nil {
		t.Fatalf("Connect() after CA update error = %v", err)
	}
	if changed.Fingerprint == connected.Fingerprint || changed.Menu == connected.Menu {
		t.Fatal("changed CA Secret reused its adoption identity or REST client")
	}
}

func selfSignedCertificate(t *testing.T) []byte {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "router.example"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	if err != nil {
		t.Fatalf("create test certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestProviderConfigConnectorRejectsInvalidConfiguration(t *testing.T) {
	short := metav1.Duration{Duration: 4 * time.Second}
	tests := map[string]func(*providerv1alpha1.ProviderConfig){
		"short timeout": func(pc *providerv1alpha1.ProviderConfig) { pc.Spec.RequestTimeout = &short },
		"TLS on HTTP": func(pc *providerv1alpha1.ProviderConfig) {
			pc.Spec.Endpoint = "http://router.example"
			pc.Spec.TLS = &providerv1alpha1.ProviderTLSConfig{}
		},
		"contradictory TLS": func(pc *providerv1alpha1.ProviderConfig) {
			pc.Spec.TLS = &providerv1alpha1.ProviderTLSConfig{
				InsecureSkipVerify: true,
				CASecretRef:        &xpv2.LocalSecretKeySelector{Name: "router-ca", Key: "ca.crt"},
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			scheme := newTestScheme(t)
			pc := newProviderConfig("tenant-a", "https://router.example")
			mutate(pc)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "router-creds", Namespace: "tenant-a"},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("secret")},
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pc, secret).Build()
			if _, err := (&ProviderConfigConnector{}).Connect(context.Background(), reader, "tenant-a", "router"); err == nil {
				t.Fatal("Connect() accepted invalid ProviderConfig")
			}
		})
	}
}

func TestFingerprintTargetCanonicalizesURL(t *testing.T) {
	base := fingerprintTarget("HTTPS://Router.Example/rest/")
	if got := fingerprintTarget("https://router.example/rest"); got != base {
		t.Fatalf("equivalent target fingerprints differ: %q != %q", got, base)
	}
	if got := fingerprintTarget("https://other.example/rest"); got == base {
		t.Fatal("different RouterOS targets have the same fingerprint")
	}
}

func TestProviderConfigConnectorCannotCrossNamespaces(t *testing.T) {
	scheme := newTestScheme(t)
	pc := newProviderConfig("tenant-a", "https://router.example")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "router-creds", Namespace: "tenant-b"},
		Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("secret")},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pc, secret).Build()

	if _, err := (&ProviderConfigConnector{}).Connect(context.Background(), reader, "tenant-b", "router"); err == nil {
		t.Fatal("Connect() resolved a ProviderConfig from another namespace")
	}
	if _, err := (&ProviderConfigConnector{}).Connect(context.Background(), reader, "tenant-a", "router"); err == nil {
		t.Fatal("Connect() resolved a credentials Secret from another namespace")
	}
}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := providerv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func newProviderConfig(namespace, endpoint string) *providerv1alpha1.ProviderConfig {
	return &providerv1alpha1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "router", Namespace: namespace},
		Spec: providerv1alpha1.ProviderConfigSpec{
			Endpoint: endpoint,
			Credentials: providerv1alpha1.ProviderCredentials{SecretRef: providerv1alpha1.ProviderCredentialSecretReference{
				Name: "router-creds",
			}},
		},
	}
}
