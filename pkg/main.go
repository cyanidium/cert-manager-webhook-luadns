package main

import (
	"context"
	"encoding/json"
	"fmt"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"os"
	"strings"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/luadns/luadns-go"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	klog.InitFlags(nil)
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	// This will register our custom DNS provider with the webhook serving
	// library, making it available as an API under the provided GroupName.
	// You can register multiple DNS provider implementations with a single
	// webhook, where the Name() method will be used to disambiguate between
	// the different implementations.
	cmd.RunWebhookServer(GroupName,
		&luaDNSProviderSolver{},
	)
}

// luaDNSProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for your own DNS provider.
// To do so, it must implement the `github.com/cert-manager/cert-manager/pkg/acme/webhook.Solver`
// interface.
type luaDNSProviderSolver struct {
	// If a Kubernetes 'clientset' is needed, you must:
	// 1. uncomment the additional `client` field in this structure below
	// 2. uncomment the "k8s.io/client-go/kubernetes" import at the top of the file
	// 3. uncomment the relevant code in the Initialize method below
	// 4. ensure your webhook's service account has the required RBAC role
	//    assigned to it for interacting with the Kubernetes APIs you need.
	client *kubernetes.Clientset
}

// luaDNSProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
// This typically includes references to Secret resources containing DNS
// provider credentials, in cases where a 'multi-tenant' DNS solver is being
// created.
// If you do *not* require per-issuer or per-certificate configuration to be
// provided to your webhook, you can skip decoding altogether in favour of
// using CLI flags or similar to provide configuration.
// You should not include sensitive information here. If credentials need to
// be used by your provider here, you should reference a Kubernetes Secret
// resource and fetch these credentials using a Kubernetes clientset.
type luaDNSProviderConfig struct {
	// Change the two fields below according to the format of the configuration
	// to be decoded.
	// These fields will be set by users in the
	// `issuer.spec.acme.dns01.providers.webhook.config` field.

	APIKeySecretRef cmmeta.SecretKeySelector `json:"apiKeySecretRef"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (c *luaDNSProviderSolver) Name() string {
	return "luadns"
}

func (c *luaDNSProviderSolver) getClient(cfg *luaDNSProviderConfig, ctx context.Context, namespace string) (client *luadns.Client, err error) {
	secretName := cfg.APIKeySecretRef.LocalObjectReference.Name // pragma: allowlist secret
	klog.Infof("Try to load secret %s.%s", secretName, cfg.APIKeySecretRef.Key)
	sec, err := c.client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})

	if err != nil {
		return nil, fmt.Errorf("unable to get secret `%s`; %v", secretName, err)
	}

	secBytes, ok := sec.Data[cfg.APIKeySecretRef.Key]

	if !ok {
		return nil, fmt.Errorf("key %q not found in secret \"%s/%s\"", cfg.APIKeySecretRef.Key, cfg.APIKeySecretRef.LocalObjectReference.Name, namespace)
	}

	apiKey := string(secBytes)

	// new client
	client = luadns.NewClient("", apiKey)

	return client, nil
}

func (c *luaDNSProviderSolver) getZone(client *luadns.Client, ctx context.Context, zoneName string) (*luadns.Zone, error) {
	klog.Infof("Looking for zone %s", zoneName)
	zones, err := client.ListZones(ctx, &luadns.ListParams{Query: zoneName})
	if err != nil {
		return nil, fmt.Errorf("unable to get zones: %s", err)
	}

	for _, z := range zones {
		if z.Name == zoneName {
			klog.Infof("Found existing zone %s", zoneName)
			return z, nil
		}
		klog.Infof("Ignoring zone %s", z.Name)
	}
	return nil, nil
}

func (c *luaDNSProviderSolver) getExistingRecord(ch *v1alpha1.ChallengeRequest, ctx context.Context) (*luadns.Client, *luadns.Zone, *luadns.Record, error) {
	cfg, err := loadConfig(ch.Config)
	domain := strings.TrimSuffix(ch.ResolvedZone, ".")
	if err != nil {
		return nil, nil, nil, err
	}

	client, err := c.getClient(&cfg, ctx, ch.ResourceNamespace)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to get client: %s", err)
	}

	zone, err := c.getZone(client, ctx, domain)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to find zone: %s", err)
	}

	klog.Infof("Looking for fqdn %s", ch.ResolvedFQDN)
	records, err := client.ListRecords(ctx, zone, &luadns.ListParams{Query: ch.ResolvedFQDN})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to get records from zone=%s: %s", zone.Name, err)
	}

	for _, r := range records {
		if r.Type == luadns.TypeTXT && r.Name == ch.ResolvedFQDN && r.Content == ch.Key {
			klog.Infof("Found existing record type=%s name=%s content=%s zone=%s", r.Type, r.Name, r.Content, zone.Name)
			return client, zone, r, nil
		}
		klog.Infof("Ignoring record type=%s name=%s content=%s zone=%s", r.Type, r.Name, r.Content, zone.Name)
	}
	return client, zone, nil, nil
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (c *luaDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	ctx := context.Background()
	klog.Infof("Present for fqdn %s", ch.ResolvedFQDN)

	client, zone, existingRecord, err := c.getExistingRecord(ch, ctx)
	if err != nil {
		return fmt.Errorf("unable to find txt records: %s", err)
	}

	if existingRecord == nil {
		// create TXT record
		record := &luadns.Record{
			Type:    luadns.TypeTXT,
			Name:    ch.ResolvedFQDN,
			Content: ch.Key,
			TTL:     60,
		}

		klog.Infof("Creating new record %s", record.Name)
		_, err = client.CreateRecord(ctx, zone, record)
		if err != nil {
			return fmt.Errorf("unable to create record: %s", err)
		}
	} else {
		klog.Infof("Record already exists %s", existingRecord.Name)
		return nil
	}

	return nil
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (c *luaDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	ctx := context.Background()
	klog.Infof("Cleanup for fqdn %s", ch.ResolvedFQDN)

	client, zone, existingRecord, err := c.getExistingRecord(ch, ctx)
	if err != nil {
		return fmt.Errorf("unable to find txt records: %s", err)
	}

	if existingRecord != nil {
		_, err = client.DeleteRecord(ctx, zone, existingRecord.ID)
		if err != nil {
			return fmt.Errorf("unable to delete record: %s", err)
		}
	} else {
		klog.Infof("No existing record to clean up for fqdn %s", ch.ResolvedFQDN)
		return nil
	}
	return nil
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (c *luaDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	c.client = cl
	return nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (luaDNSProviderConfig, error) {
	cfg := luaDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}
