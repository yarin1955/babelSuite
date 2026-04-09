package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/catalog"
	"github.com/babelsuite/babelsuite/internal/suites"
)

const (
	ociImageManifestMediaType = "application/vnd.oci.image.manifest.v1+json"
	ociImageConfigMediaType   = "application/vnd.oci.image.config.v1+json"
)

type repositoryTag struct {
	Repository  string
	Tag         string
	Kind        string
	SourceFiles []suites.SourceFile
}

type catalogResponse struct {
	Repositories []string `json:"repositories"`
}

func main() {
	registryURL := flag.String("registry", "http://localhost:5000", "Base URL for the Zot registry")
	includeModules := flag.Bool("include-modules", true, "Also publish seeded stdlib modules")
	flag.Parse()

	publisher, err := newPublisher(*registryURL)
	if err != nil {
		fatalf("normalize registry: %v", err)
	}

	references := seedReferences(*includeModules)
	for _, reference := range references {
		if err := publisher.publish(reference); err != nil {
			fatalf("publish %s:%s: %v", reference.Repository, reference.Tag, err)
		}
		fmt.Printf("Published %s:%s\n", reference.Repository, reference.Tag)
	}

	repositories, err := publisher.listRepositories()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: could not list catalog immediately: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("Zot catalog contents:")
	body, _ := json.Marshal(catalogResponse{Repositories: repositories})
	fmt.Println(string(body))
}

type publisher struct {
	baseURL *url.URL
	client  *http.Client
}

func newPublisher(raw string) (*publisher, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("registry URL is empty")
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("registry URL host is empty")
	}

	return &publisher{
		baseURL: parsed,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (p *publisher) publish(reference repositoryTag) error {
	var layerBlob []byte
	var layerDigest, diffID string

	if len(reference.SourceFiles) > 0 {
		var err error
		layerBlob, layerDigest, diffID, err = buildLayer(reference.SourceFiles)
		if err != nil {
			return fmt.Errorf("build layer: %w", err)
		}
		if err := p.ensureBlob(reference.Repository, layerDigest, layerBlob); err != nil {
			return err
		}
	}

	configBody, configDigest, err := imageConfig(reference, diffID)
	if err != nil {
		return err
	}
	if err := p.ensureBlob(reference.Repository, configDigest, configBody); err != nil {
		return err
	}

	manifestBody, err := imageManifest(reference, configDigest, len(configBody), layerDigest, len(layerBlob))
	if err != nil {
		return err
	}
	return p.putManifest(reference.Repository, reference.Tag, manifestBody)
}

func (p *publisher) ensureBlob(repository, digest string, body []byte) error {
	target := p.endpoint("/v2/" + encodeRepositoryPath(repository) + "/blobs/" + digest)
	req, err := http.NewRequest(http.MethodHead, target, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
	}

	startURL := p.endpoint("/v2/" + encodeRepositoryPath(repository) + "/blobs/uploads/")
	startReq, err := http.NewRequest(http.MethodPost, startURL, nil)
	if err != nil {
		return err
	}
	startResp, err := p.client.Do(startReq)
	if err != nil {
		return err
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("blob upload init returned %s", startResp.Status)
	}

	location := startResp.Header.Get("Location")
	if strings.TrimSpace(location) == "" {
		return fmt.Errorf("blob upload init did not return a location")
	}
	uploadURL, err := p.resolveLocation(location)
	if err != nil {
		return err
	}

	query := uploadURL.Query()
	query.Set("digest", digest)
	uploadURL.RawQuery = query.Encode()

	uploadReq, err := http.NewRequest(http.MethodPut, uploadURL.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	uploadResp, err := p.client.Do(uploadReq)
	if err != nil {
		return err
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusCreated && uploadResp.StatusCode != http.StatusAccepted {
		message, _ := io.ReadAll(uploadResp.Body)
		return fmt.Errorf("blob upload returned %s: %s", uploadResp.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

func (p *publisher) putManifest(repository, tag string, body []byte) error {
	target := p.endpoint("/v2/" + encodeRepositoryPath(repository) + "/manifests/" + url.PathEscape(tag))
	req, err := http.NewRequest(http.MethodPut, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", ociImageManifestMediaType)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		message, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("manifest upload returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	return nil
}

func (p *publisher) listRepositories() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, p.endpoint("/v2/_catalog?n=1000"), nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog request returned %s", resp.Status)
	}

	var payload catalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	sort.Strings(payload.Repositories)
	return payload.Repositories, nil
}

func (p *publisher) resolveLocation(raw string) (*url.URL, error) {
	location, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	return p.baseURL.ResolveReference(location), nil
}

func (p *publisher) endpoint(path string) string {
	resolved := *p.baseURL
	resolved.Path = strings.TrimRight(p.baseURL.Path, "/") + path
	resolved.RawQuery = ""
	return resolved.String()
}

func imageConfig(reference repositoryTag, diffID string) ([]byte, string, error) {
	diffIDs := []string{}
	if diffID != "" {
		diffIDs = []string{diffID}
	}
	config := map[string]any{
		"created":      time.Now().UTC().Format(time.RFC3339Nano),
		"architecture": "amd64",
		"os":           "linux",
		"config": map[string]any{
			"Labels": map[string]string{
				"io.babelsuite.seed":                "true",
				"io.babelsuite.kind":                reference.Kind,
				"org.opencontainers.image.ref.name": reference.Tag,
				"org.opencontainers.image.title":    reference.Repository,
			},
		},
		"rootfs": map[string]any{
			"type":     "layers",
			"diff_ids": diffIDs,
		},
	}

	body, err := json.Marshal(config)
	if err != nil {
		return nil, "", err
	}
	return body, digest(body), nil
}

func imageManifest(reference repositoryTag, configDigest string, configSize int, layerDigest string, layerSize int) ([]byte, error) {
	layers := []any{}
	if layerDigest != "" && layerSize > 0 {
		layers = []any{
			map[string]any{
				"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
				"size":      layerSize,
				"digest":    layerDigest,
			},
		}
	}
	manifest := map[string]any{
		"schemaVersion": 2,
		"mediaType":     ociImageManifestMediaType,
		"config": map[string]any{
			"mediaType": ociImageConfigMediaType,
			"size":      configSize,
			"digest":    configDigest,
		},
		"layers": layers,
		"annotations": map[string]string{
			"io.babelsuite.seed":               "true",
			"org.opencontainers.image.title":   reference.Repository,
			"org.opencontainers.image.version": reference.Tag,
		},
	}
	return json.Marshal(manifest)
}

// buildLayer tars the suite source files and gzip-compresses them into an OCI
// layer blob. Returns the compressed blob, its sha256 digest (for the manifest
// layers array), and the uncompressed digest (diff_id for the image config).
func buildLayer(files []suites.SourceFile) (blob []byte, layerDigest, diffID string, err error) {
	// Build the uncompressed tar first so we can capture the diff_id.
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for _, f := range files {
		content := []byte(f.Content)
		hdr := &tar.Header{
			Name:     f.Path,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
			ModTime:  time.Unix(0, 0).UTC(),
		}
		if err = tw.WriteHeader(hdr); err != nil {
			return
		}
		if _, err = tw.Write(content); err != nil {
			return
		}
	}
	if err = tw.Close(); err != nil {
		return
	}

	rawSum := sha256.Sum256(tarBuf.Bytes())
	diffID = "sha256:" + hex.EncodeToString(rawSum[:])

	// Gzip-compress for the actual layer blob.
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err = io.Copy(gw, &tarBuf); err != nil {
		return
	}
	if err = gw.Close(); err != nil {
		return
	}

	blob = gzBuf.Bytes()
	gzSum := sha256.Sum256(blob)
	layerDigest = "sha256:" + hex.EncodeToString(gzSum[:])
	return
}

func seedReferences(includeModules bool) []repositoryTag {
	seen := make(map[string]struct{})
	references := make([]repositoryTag, 0)

	for _, suiteDefinition := range suites.NewService().List() {
		repository := repositoryPath(suiteDefinition.Repository)
		for _, tag := range suiteDefinition.Tags {
			references = appendReference(references, seen, repository, tag, "suite", suiteDefinition.SourceFiles)
		}
	}

	if includeModules {
		for _, module := range catalog.SeedStdlibPackages() {
			repository := repositoryPath(module.Repository)
			for _, tag := range module.Tags {
				references = appendReference(references, seen, repository, tag, "stdlib", nil)
			}
		}
	}

	sort.Slice(references, func(i, j int) bool {
		if references[i].Repository != references[j].Repository {
			return references[i].Repository < references[j].Repository
		}
		return references[i].Tag < references[j].Tag
	})
	return references
}

func appendReference(references []repositoryTag, seen map[string]struct{}, repository, tag, kind string, sourceFiles []suites.SourceFile) []repositoryTag {
	repository = strings.Trim(repository, "/")
	tag = strings.TrimSpace(tag)
	if repository == "" || tag == "" {
		return references
	}
	key := repository + ":" + tag
	if _, ok := seen[key]; ok {
		return references
	}
	seen[key] = struct{}{}
	return append(references, repositoryTag{
		Repository:  repository,
		Tag:         tag,
		Kind:        kind,
		SourceFiles: sourceFiles,
	})
}

func repositoryPath(repository string) string {
	repository = strings.TrimSpace(repository)
	if repository == "" {
		return ""
	}
	if strings.Contains(repository, "://") {
		if parsed, err := url.Parse(repository); err == nil {
			return strings.Trim(parsed.Path, "/")
		}
	}
	if slash := strings.Index(repository, "/"); slash >= 0 {
		host := repository[:slash]
		if strings.Contains(host, ".") || strings.Contains(host, ":") || strings.EqualFold(host, "localhost") {
			return strings.Trim(repository[slash+1:], "/")
		}
	}
	return strings.Trim(repository, "/")
}

func encodeRepositoryPath(repository string) string {
	parts := strings.Split(strings.Trim(repository, "/"), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
