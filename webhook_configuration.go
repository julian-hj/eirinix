package extension

import (
	"context"
	"encoding/base64"
	"net"
	"net/url"
	"os"
	"path"
	"strconv"

	"github.com/pkg/errors"
	"github.com/spf13/afero"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	machinerytypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"code.cloudfoundry.org/cf-operator/pkg/credsgen"
	"code.cloudfoundry.org/cf-operator/pkg/kube/util/config"
	"github.com/SUSE/eirinix/util/ctxlog"
)

// WebhookConfig generates certificates and the configuration for the webhook server
type WebhookConfig struct {
	ConfigName    string
	CertDir       string
	Certificate   []byte
	Key           []byte
	CaCertificate []byte
	CaKey         []byte

	setupCertificateName string
	client               client.Client
	config               *config.Config
	generator            credsgen.Generator
}

// NewWebhookConfig returns a new WebhookConfig
func NewWebhookConfig(c client.Client, config *config.Config, generator credsgen.Generator, configName string, setupCertificateName string) *WebhookConfig {
	return &WebhookConfig{
		ConfigName:           configName,
		CertDir:              path.Join(os.TempDir(), setupCertificateName),
		client:               c,
		config:               config,
		generator:            generator,
		setupCertificateName: setupCertificateName,
	}
}

// SetupCertificate ensures that a CA and a certificate is available for the
// webhook server
func (f *WebhookConfig) setupCertificate(ctx context.Context) error {
	secretNamespacedName := machinerytypes.NamespacedName{
		Name:      f.setupCertificateName,
		Namespace: f.config.Namespace,
	}

	// We have to query for the Secret using an unstructured object because the cache for the structured
	// client is not initialized yet at this point in time. See https://github.com/kubernetes-sigs/controller-runtime/issues/180
	secret := &unstructured.Unstructured{}
	secret.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Kind:    "Secret",
		Version: "v1",
	})
	f.client.Get(ctx, secretNamespacedName, secret)

	if secret.GetName() != "" {
		ctxlog.Info(ctx, "Not creating the webhook server certificate because it already exists")
		data := secret.Object["data"].(map[string]interface{})
		caKey, err := base64.StdEncoding.DecodeString(data["ca_private_key"].(string))
		if err != nil {
			return err
		}
		caCert, err := base64.StdEncoding.DecodeString(data["ca_certificate"].(string))
		if err != nil {
			return err
		}
		key, err := base64.StdEncoding.DecodeString(data["private_key"].(string))
		if err != nil {
			return err
		}
		cert, err := base64.StdEncoding.DecodeString(data["certificate"].(string))
		if err != nil {
			return err
		}

		f.CaKey = caKey
		f.CaCertificate = caCert
		f.Key = key
		f.Certificate = cert
	} else {
		ctxlog.Info(ctx, "Creating webhook server certificate")

		// Generate CA
		caRequest := credsgen.CertificateGenerationRequest{
			CommonName:       "SCF CA",
			IsCA:             true,
			AlternativeNames: []string{f.config.WebhookServerHost},
		}

		caCert, err := f.generator.GenerateCertificate("webhook-server-ca", caRequest)
		if err != nil {
			return err
		}

		// Generate Certificate
		request := credsgen.CertificateGenerationRequest{
			IsCA:       false,
			CommonName: f.config.WebhookServerHost,
			CA: credsgen.Certificate{
				IsCA:        true,
				PrivateKey:  caCert.PrivateKey,
				Certificate: caCert.Certificate,
			},
		}
		cert, err := f.generator.GenerateCertificate("webhook-server-cert", request)
		if err != nil {
			return err
		}

		newSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretNamespacedName.Name,
				Namespace: secretNamespacedName.Namespace,
			},
			Data: map[string][]byte{
				"certificate":    cert.Certificate,
				"private_key":    cert.PrivateKey,
				"ca_certificate": caCert.Certificate,
				"ca_private_key": caCert.PrivateKey,
			},
		}
		err = f.client.Create(ctx, newSecret)
		if err != nil {
			return err
		}

		f.CaKey = caCert.PrivateKey
		f.CaCertificate = caCert.Certificate
		f.Key = cert.PrivateKey
		f.Certificate = cert.Certificate
	}

	err := f.writeSecretFiles()
	if err != nil {
		return errors.Wrap(err, "writing webhook certificate files to disk")
	}

	return nil
}

func (f *WebhookConfig) generateWebhookServerConfig(ctx context.Context, webhooks []*admission.Webhook) error {
	if len(f.CaCertificate) == 0 {
		return errors.New("Can not create a webhook server config with an empty ca certificate")
	}

	config := &admissionregistrationv1beta1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.ConfigName,
			Namespace: f.config.Namespace,
		},
	}

	for _, webhook := range webhooks {
		url := url.URL{
			Scheme: "https",
			Host:   net.JoinHostPort(f.config.WebhookServerHost, strconv.Itoa(int(f.config.WebhookServerPort))),
			Path:   webhook.Path,
		}
		urlString := url.String()
		wh := admissionregistrationv1beta1.Webhook{
			Name:              webhook.GetName(),
			Rules:             webhook.Rules,
			FailurePolicy:     webhook.FailurePolicy,
			NamespaceSelector: webhook.NamespaceSelector,
			ClientConfig: admissionregistrationv1beta1.WebhookClientConfig{
				CABundle: f.CaCertificate,
				URL:      &urlString,
			},
		}

		config.Webhooks = append(config.Webhooks, wh)
	}

	f.client.Delete(ctx, config)
	err := f.client.Create(ctx, config)
	if err != nil {
		return errors.Wrap(err, "generating the webhook configuration")
	}

	return nil
}

func (f *WebhookConfig) writeSecretFiles() error {
	if exists, _ := afero.DirExists(f.config.Fs, f.CertDir); !exists {
		err := f.config.Fs.Mkdir(f.CertDir, 0700)
		if err != nil {
			return err
		}
	}

	err := afero.WriteFile(f.config.Fs, path.Join(f.CertDir, "ca-key.pem"), f.CaKey, 0600)
	if err != nil {
		return err
	}
	err = afero.WriteFile(f.config.Fs, path.Join(f.CertDir, "ca-cert.pem"), f.CaCertificate, 0644)
	if err != nil {
		return err
	}
	err = afero.WriteFile(f.config.Fs, path.Join(f.CertDir, "key.pem"), f.Key, 0600)
	if err != nil {
		return err
	}
	err = afero.WriteFile(f.config.Fs, path.Join(f.CertDir, "cert.pem"), f.Certificate, 0644)
	if err != nil {
		return err
	}
	return nil
}
