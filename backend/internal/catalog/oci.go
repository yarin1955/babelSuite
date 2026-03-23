package catalog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/domain"
	"github.com/google/uuid"
)

type ociClient struct {
	baseURL string
	token   string
	http    *http.Client
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
	base := registry.URL
	if base == "" {
		switch registry.Kind {
		case domain.RegistryGHCR:
			base = "https://ghcr.io"
		case domain.RegistryJFrog:
			base = "https://your-org.jfrog.io"
		}
	}
	base = strings.TrimRight(base, "/")
	return &ociClient{
		baseURL: base,
		token:   registry.Token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *ociClient) do(method, path string) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
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
		desc  := ann["org.opencontainers.image.description"]
		ver   := ann["org.opencontainers.image.version"]
		if ver == "" {
			ver = tag
		}
		pub   := ann["io.babelsuite.publisher"]
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

		pkgs = append(pkgs, &domain.CatalogPackage{
			PackageID:    uuid.NewString(),
			OrgID:        orgID,
			RegistryID:   registry.RegistryID,
			RegistryKind: registry.Kind,
			Name:         name,
			DisplayName:  ann["org.opencontainers.image.title"],
			Description:  desc,
			Publisher:    pub,
			ImageRef:     fmt.Sprintf("%s/%s:%s", strings.TrimPrefix(strings.TrimPrefix(registry.URL, "https://"), "http://"), repo, tag),
			Version:      ver,
			Tags:         tags,
			Enabled:      false,
			UpdatedAt:    time.Now().UTC(),
		})
	}
	return pkgs, nil
}
