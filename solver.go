package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const defaultAPIBase = "https://api.name.com"

type namedotcomSolver struct {
	client     kubernetes.Interface
	httpClient *http.Client
}

type namedotcomConfig struct {
	// APIURLBase overrides the name.com API base URL. Leave empty for production.
	// Set to "https://api.dev.name.com" for the name.com test environment.
	APIURLBase string `json:"apiURLBase"`

	// DomainName overrides the computed registered domain (e.g. "irwin.earth").
	// Only needed if domainFromZone produces the wrong result for your zone.
	DomainName string `json:"domainName"`

	UsernameSecretRef corev1.SecretKeySelector `json:"usernameSecretRef"`
	APITokenSecretRef corev1.SecretKeySelector `json:"apiTokenSecretRef"`
}

// namecomRecord mirrors the name.com API record object.
type namecomRecord struct {
	ID         int32  `json:"id,omitempty"`
	DomainName string `json:"domainName,omitempty"`
	Host       string `json:"host"`
	Type       string `json:"type"`
	Answer     string `json:"answer"`
	TTL        uint32 `json:"ttl"`
}

type listRecordsResponse struct {
	Records  []*namecomRecord `json:"records"`
	NextPage int32            `json:"nextPage"`
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
	s.httpClient = &http.Client{Timeout: 30 * time.Second}
	return nil
}

func (s *namedotcomSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	username, token, err := s.loadCredentials(cfg, ch.ResourceNamespace)
	if err != nil {
		return err
	}
	host, domain := resolveHostAndDomain(cfg, string(ch.ResolvedFQDN), string(ch.ResolvedZone))
	apiBase := apiBaseURL(cfg.APIURLBase)

	record := &namecomRecord{
		Host:   host,
		Type:   "TXT",
		Answer: ch.Key,
		TTL:    300,
	}
	body, _ := json.Marshal(record)
	url := fmt.Sprintf("%s/v4/domains/%s/records", apiBase, domain)
	req, err := http.NewRequestWithContext(context.TODO(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("namedotcom webhook: building create request: %w", err)
	}
	req.SetBasicAuth(username, token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("namedotcom webhook: creating TXT record: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("namedotcom webhook: create record returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (s *namedotcomSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}
	username, token, err := s.loadCredentials(cfg, ch.ResourceNamespace)
	if err != nil {
		return err
	}
	host, domain := resolveHostAndDomain(cfg, string(ch.ResolvedFQDN), string(ch.ResolvedZone))
	apiBase := apiBaseURL(cfg.APIURLBase)

	page := int32(1)
	for {
		url := fmt.Sprintf("%s/v4/domains/%s/records?page=%d", apiBase, domain, page)
		req, err := http.NewRequestWithContext(context.TODO(), http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("namedotcom webhook: building list request: %w", err)
		}
		req.SetBasicAuth(username, token)

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("namedotcom webhook: listing records for %s: %w", domain, err)
		}
		var listResp listRecordsResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			resp.Body.Close()
			return fmt.Errorf("namedotcom webhook: decoding list response: %w", err)
		}
		resp.Body.Close()

		for _, r := range listResp.Records {
			if r.Type == "TXT" && r.Host == host && r.Answer == ch.Key {
				delURL := fmt.Sprintf("%s/v4/domains/%s/records/%d", apiBase, domain, r.ID)
				delReq, err := http.NewRequestWithContext(context.TODO(), http.MethodDelete, delURL, nil)
				if err != nil {
					return fmt.Errorf("namedotcom webhook: building delete request: %w", err)
				}
				delReq.SetBasicAuth(username, token)
				delResp, err := s.httpClient.Do(delReq)
				if err != nil {
					return fmt.Errorf("namedotcom webhook: deleting record %d: %w", r.ID, err)
				}
				delResp.Body.Close()
				if delResp.StatusCode < 200 || delResp.StatusCode >= 300 {
					return fmt.Errorf("namedotcom webhook: delete record %d returned HTTP %d", r.ID, delResp.StatusCode)
				}
			}
		}

		if listResp.NextPage == 0 {
			break
		}
		page = listResp.NextPage
	}
	return nil
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

// resolveHostAndDomain returns the host and registered domain for a name.com API call,
// respecting the optional DomainName override in cfg.
func resolveHostAndDomain(cfg namedotcomConfig, fqdn, zone string) (host, domain string) {
	host, domain = extractHostAndDomain(fqdn, zone)
	if cfg.DomainName != "" {
		domain = cfg.DomainName
		fqdn = strings.TrimSuffix(fqdn, ".")
		host = strings.TrimSuffix(fqdn, "."+domain)
	}
	return host, domain
}

func apiBaseURL(configured string) string {
	if configured == "" {
		return defaultAPIBase
	}
	return strings.TrimSuffix(configured, "/")
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
