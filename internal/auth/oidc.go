package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// OIDCDiscovery holds the OIDC discovery document fields we need.
type OIDCDiscovery struct {
	Issuer                string `json:"issuer"`
	JWKSURI               string `json:"jwks_uri"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// OIDCProvider manages OIDC provider discovery and configuration.
type OIDCProvider struct {
	issuerURL  string
	discovery  *OIDCDiscovery
	mu         sync.RWMutex
	httpClient *http.Client
}

// NewOIDCProvider creates a new OIDC provider with the given issuer URL.
func NewOIDCProvider(issuerURL string) *OIDCProvider {
	return &OIDCProvider{
		issuerURL: issuerURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Discover fetches the OIDC discovery document from the well-known endpoint.
func (p *OIDCProvider) Discover(ctx context.Context) (*OIDCDiscovery, error) {
	p.mu.RLock()
	if p.discovery != nil {
		d := p.discovery
		p.mu.RUnlock()
		return d, nil
	}
	p.mu.RUnlock()

	url := p.issuerURL + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating discovery request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery endpoint returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading discovery response: %w", err)
	}

	var disc OIDCDiscovery
	if err := json.Unmarshal(body, &disc); err != nil {
		return nil, fmt.Errorf("parsing discovery document: %w", err)
	}

	p.mu.Lock()
	p.discovery = &disc
	p.mu.Unlock()

	return &disc, nil
}

// IssuerURL returns the configured issuer URL.
func (p *OIDCProvider) IssuerURL() string {
	return p.issuerURL
}
