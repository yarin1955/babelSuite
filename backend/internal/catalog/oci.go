package catalog

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/google/uuid"
)

type ociClient struct {
	baseURL  string
	username string
	password string
	token    string
	http     *http.Client
}

type tagList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type ociManifest struct {
	Annotations map[string]string `json:"annotations"`
	Config      struct {
		MediaType string `json:"mediaType"`
	} `json:"config"`
}

func newOCIClient(registry *domain.Registry) *ociClient {
	base := resolveRegistryBaseURL(registry)
	httpClient := &http.Client{Timeout: 15 * time.Second}
	if transport, err := registryHTTPTransport(registry); err == nil && transport != nil {
		httpClient.Transport = transport
	}

	return &ociClient{
		baseURL:  strings.TrimRight(base, "/"),
		username: strings.TrimSpace(registry.Username),
		password: registry.Password,
		token:    registry.Token,
		http:     httpClient,
	}
}

func (c *ociClient) do(method, path string) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.username != "" && c.password != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.username+":"+c.password)))
	} else if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
	return c.http.Do(req)
}

func (c *ociClient) ListTags(repo string) ([]string, error) {
	resp, err := c.do("GET", fmt.Sprintf("/v2/%s/tags/list", repo))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned %d for %s", resp.StatusCode, repo)
	}
	var tl tagList
	if err := json.NewDecoder(resp.Body).Decode(&tl); err != nil {
		return nil, err
	}
	return tl.Tags, nil
}

func (c *ociClient) GetManifest(repo, ref string) (*ociManifest, error) {
	resp, err := c.do("GET", fmt.Sprintf("/v2/%s/manifests/%s", repo, ref))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned %d for manifest %s:%s", resp.StatusCode, repo, ref)
	}
	var m ociManifest
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// SyncRepo fetches all tags from an OCI repository and returns CatalogPackage
// entries with metadata read from OCI annotations.
func SyncRepo(registry *domain.Registry, orgID, repo string) ([]*domain.CatalogPackage, error) {
	client := newOCIClient(registry)

	tags, err := client.ListTags(repo)
	if err != nil {
		return nil, err
	}

	var pkgs []*domain.CatalogPackage
	for _, tag := range tags {
		manifest, err := client.GetManifest(repo, tag)
		if err != nil {
			continue
		}
		ann := manifest.Annotations

		name := ann["org.opencontainers.image.title"]
		if name == "" {
			parts := strings.Split(repo, "/")
			name = parts[len(parts)-1]
		}
		desc := ann["org.opencontainers.image.description"]
		ver := ann["org.opencontainers.image.version"]
		if ver == "" {
			ver = tag
		}
		pub := ann["io.babelsuite.publisher"]
		if pub == "" {
			pub = ann["org.opencontainers.image.vendor"]
		}
		rawTags := ann["io.babelsuite.tags"]
		var tags []string
		if rawTags != "" {
			for _, t := range strings.Split(rawTags, ",") {
				if s := strings.TrimSpace(t); s != "" {
					tags = append(tags, s)
				}
			}
		}
		rawProfiles := ann["io.babelsuite.profiles"]
		var profiles []string
		if rawProfiles != "" {
			for _, profile := range strings.Split(rawProfiles, ",") {
				if s := strings.TrimSpace(profile); s != "" {
					profiles = append(profiles, s)
				}
			}
		}
		defaultProfile := strings.TrimSpace(ann["io.babelsuite.default_profile"])
		if defaultProfile != "" && !containsString(profiles, defaultProfile) {
			profiles = append([]string{defaultProfile}, profiles...)
		}

		pkgs = append(pkgs, &domain.CatalogPackage{
			PackageID:      uuid.NewString(),
			OrgID:          orgID,
			RegistryID:     registry.RegistryID,
			RegistryKind:   registry.Kind,
			Name:           name,
			DisplayName:    ann["org.opencontainers.image.title"],
			Description:    desc,
			Publisher:      pub,
			ImageRef:       fmt.Sprintf("%s/%s:%s", registryImageHost(registry), repo, tag),
			Version:        ver,
			Tags:           tags,
			Profiles:       profiles,
			DefaultProfile: defaultProfile,
			Enabled:        false,
			UpdatedAt:      time.Now().UTC(),
		})
	}
	return pkgs, nil
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func resolveRegistryBaseURL(registry *domain.Registry) string {
	base := strings.TrimSpace(registry.URL)
	if base != "" {
		return base
	}
	switch registry.Kind {
	case domain.RegistryGHCR:
		return "https://ghcr.io"
	case domain.RegistryJFrog:
		return "https://your-org.jfrog.io"
	default:
		return ""
	}
}

func registryImageHost(registry *domain.Registry) string {
	base := resolveRegistryBaseURL(registry)
	if base == "" {
		return ""
	}
	if parsed, err := url.Parse(base); err == nil && parsed.Host != "" {
		host := parsed.Host
		path := strings.Trim(parsed.Path, "/")
		if path != "" {
			return host + "/" + path
		}
		return host
	}
	return strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")
}

func registryHTTPTransport(registry *domain.Registry) (*http.Transport, error) {
	hasTLSConfig := registry.InsecureSkipTLSVerify ||
		strings.TrimSpace(registry.TLSCAData) != "" ||
		strings.TrimSpace(registry.TLSCertData) != "" ||
		strings.TrimSpace(registry.TLSKeyData) != ""
	if !hasTLSConfig {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: registry.InsecureSkipTLSVerify,
	}
	if caData := strings.TrimSpace(registry.TLSCAData); caData != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caData)) {
			return nil, fmt.Errorf("invalid registry tls_ca_data")
		}
		tlsConfig.RootCAs = pool
	}

	certData := strings.TrimSpace(registry.TLSCertData)
	keyData := strings.TrimSpace(registry.TLSKeyData)
	if certData != "" || keyData != "" {
		cert, err := tls.X509KeyPair([]byte(certData), []byte(keyData))
		if err != nil {
			return nil, fmt.Errorf("invalid registry client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return &http.Transport{
		TLSClientConfig: tlsConfig,
		MaxIdleConns:    6,
		IdleConnTimeout: 30 * time.Second,
	}, nil
}
