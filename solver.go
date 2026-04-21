package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	namecom "github.com/namedotcom/go/namecom"
	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type namedotcomSolver struct {
	client kubernetes.Interface
}

type namedotcomConfig struct {
	// APIURLBase overrides the name.com API base URL. Leave empty for production.
	// Set to "https://api.dev.name.com" for testing.
	APIURLBase string `json:"apiURLBase"`

	// DomainName overrides the computed registered domain (e.g. "irwin.earth").
	// Only needed if domainFromZone produces the wrong result for your zone.
	DomainName string `json:"domainName"`

	UsernameSecretRef corev1.SecretKeySelector `json:"usernameSecretRef"`
	APITokenSecretRef corev1.SecretKeySelector `json:"apiTokenSecretRef"`
}

func (s *namedotcomSolver) Name() string {
	return "namedotcom"
}

func (s *namedotcomSolver) Initialize(kubeClientConfig *rest.Config, _ <-chan struct{}) error {
	cl, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return fmt.Errorf("namedotcom webhook: failed to create kubernetes client: %w", err)
	}
	s.client = cl
	return nil
}

func (s *namedotcomSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	nc, host, domain, err := s.buildClient(cfg, ch)
	if err != nil {
		return err
	}
	_, err = nc.CreateRecord(&namecom.Record{
		DomainName: domain,
		Host:       host,
		Type:       "TXT",
		Answer:     ch.Key,
		TTL:        300,
	})
	return err
}

func (s *namedotcomSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	nc, host, domain, err := s.buildClient(cfg, ch)
	if err != nil {
		return err
	}

	req := &namecom.ListRecordsRequest{DomainName: domain, Page: 1}
	for {
		resp, err := nc.ListRecords(req)
		if err != nil {
			return fmt.Errorf("namedotcom webhook: listing records for %s: %w", domain, err)
		}
		for _, r := range resp.Records {
			if r.Type == "TXT" && r.Host == host && r.Answer == ch.Key {
				if _, err := nc.DeleteRecord(&namecom.DeleteRecordRequest{
					DomainName: domain,
					ID:         r.ID,
				}); err != nil {
					return fmt.Errorf("namedotcom webhook: deleting record %d: %w", r.ID, err)
				}
			}
		}
		if resp.NextPage == 0 {
			break
		}
		req.Page = resp.NextPage
	}
	return nil
}

// buildClient sets up the name.com API client and computes the host/domain for a challenge.
func (s *namedotcomSolver) buildClient(cfg namedotcomConfig, ch *v1alpha1.ChallengeRequest) (*namecom.NameCom, string, string, error) {
	username, token, err := s.loadCredentials(cfg, ch.ResourceNamespace)
	if err != nil {
		return nil, "", "", err
	}

	host, domain := extractHostAndDomain(string(ch.ResolvedFQDN), string(ch.ResolvedZone))
	if cfg.DomainName != "" {
		domain = cfg.DomainName
		fqdn := strings.TrimSuffix(string(ch.ResolvedFQDN), ".")
		host = strings.TrimSuffix(fqdn, "."+domain)
	}

	var nc *namecom.NameCom
	switch cfg.APIURLBase {
	case "", "https://api.name.com":
		nc = namecom.New(username, token)
	default:
		// Dev/test environment
		nc = namecom.New(username, token)
		nc.Server = cfg.APIURLBase
	}

	return nc, host, domain, nil
}

func (s *namedotcomSolver) loadCredentials(cfg namedotcomConfig, ns string) (username, token string, err error) {
	usernameSecret, err := s.client.CoreV1().Secrets(ns).Get(
		context.TODO(),
		cfg.UsernameSecretRef.Name,
		metav1.GetOptions{},
	)
	if err != nil {
		return "", "", fmt.Errorf("namedotcom webhook: getting username secret %s/%s: %w",
			ns, cfg.UsernameSecretRef.Name, err)
	}

	tokenSecret, err := s.client.CoreV1().Secrets(ns).Get(
		context.TODO(),
		cfg.APITokenSecretRef.Name,
		metav1.GetOptions{},
	)
	if err != nil {
		return "", "", fmt.Errorf("namedotcom webhook: getting token secret %s/%s: %w",
			ns, cfg.APITokenSecretRef.Name, err)
	}

	username = strings.TrimSpace(string(usernameSecret.Data[cfg.UsernameSecretRef.Key]))
	token = strings.TrimSpace(string(tokenSecret.Data[cfg.APITokenSecretRef.Key]))
	return username, token, nil
}

func loadConfig(cfgJSON *extapi.JSON) (namedotcomConfig, error) {
	cfg := namedotcomConfig{}
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("namedotcom webhook: decoding solver config: %w", err)
	}
	return cfg, nil
}
