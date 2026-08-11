package connection

import (
	"context"
	"testing"

	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1beta1 "github.com/sindrip/provider-routeros/apis/cluster/v1beta1"
)

func TestParseCredentials(t *testing.T) {
	tests := map[string]struct {
		raw      string
		wantHost string
		wantErr  bool
	}{
		"adds https": {
			raw:      `{"hosturl":"router.example","username":"admin","password":"secret"}`,
			wantHost: "https://router.example",
		},
		"keeps REST URL": {
			raw:      `{"hosturl":"http://router.example","rest_timeout":7}`,
			wantHost: "http://router.example",
		},
		"rejects binary API scheme": {
			raw:     `{"hosturl":"api://router.example"}`,
			wantErr: true,
		},
		"rejects contradictory TLS": {
			raw:     `{"hosturl":"router.example","insecure":true,"ca_certificate":"ca.pem"}`,
			wantErr: true,
		},
		"requires host": {raw: `{}`, wantErr: true},
		"rejects short timeout": {
			raw:     `{"hosturl":"router.example","rest_timeout":4}`,
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseCredentials([]byte(test.raw))
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseCredentials() succeeded: %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCredentials() error = %v", err)
			}
			if got.HostURL != test.wantHost {
				t.Fatalf("HostURL = %q, want %q", got.HostURL, test.wantHost)
			}
		})
	}
}

func TestProviderConfigConnectorUsesExistingSecretShape(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := clusterv1beta1.SchemeBuilder.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	pc := &clusterv1beta1.ProviderConfig{
		ObjectMeta: metav1.ObjectMeta{Name: "router"},
		Spec: clusterv1beta1.ProviderConfigSpec{Credentials: clusterv1beta1.ProviderCredentials{
			Source: xpv2.CredentialsSourceSecret,
			CommonCredentialSelectors: xpv2.CommonCredentialSelectors{SecretRef: &xpv2.SecretKeySelector{
				SecretReference: xpv2.SecretReference{Name: "router-creds", Namespace: "crossplane-system"},
				Key:             "credentials",
			}},
		}},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "router-creds", Namespace: "crossplane-system"},
		Data: map[string][]byte{
			"credentials": []byte(`{"hosturl":"https://router.example","username":"admin","password":"secret","insecure":true}`),
		},
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pc, secret).Build()

	connector := &ProviderConfigConnector{}
	connected, err := connector.Connect(context.Background(), reader, "router")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if connected.Menu == nil {
		t.Fatal("Connect() returned a nil REST menu")
	}
	if len(connected.Fingerprint) != 64 {
		t.Fatalf("connection fingerprint = %q", connected.Fingerprint)
	}
	again, err := connector.Connect(context.Background(), reader, "router")
	if err != nil {
		t.Fatalf("second Connect() error = %v", err)
	}
	if connected.Menu != again.Menu || connected.Fingerprint != again.Fingerprint {
		t.Fatal("unchanged ProviderConfig and Secret did not reuse their REST client")
	}
	updated := &corev1.Secret{}
	if err := reader.Get(context.Background(), client.ObjectKey{Name: "router-creds", Namespace: "crossplane-system"}, updated); err != nil {
		t.Fatal(err)
	}
	updated.Data["credentials"] = []byte(`{"hosturl":"https://other-router.example","username":"admin","password":"secret","insecure":true}`)
	if err := reader.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	repointed, err := connector.Connect(context.Background(), reader, "router")
	if err != nil {
		t.Fatalf("Connect() after credential update error = %v", err)
	}
	if repointed.Fingerprint == connected.Fingerprint || repointed.Menu == connected.Menu {
		t.Fatal("changed router credentials reused their adoption identity or REST client")
	}
}
