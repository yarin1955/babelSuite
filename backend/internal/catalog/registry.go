package catalog

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

func normalizeRegistryURL(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", errors.New("registry URL is empty")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", err
	}
	if parsed.Host == "" {
		return "", "", errors.New("registry URL host is empty")
	}

	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String(), parsed.Host, nil
}

func validateRegistryURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid registry URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("registry URL scheme must be http or https")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if isRegistryBlockedIP(ip) {
			return fmt.Errorf("registry URL must not target a private or reserved address")
		}
		return nil
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("registry URL host could not be resolved: %w", err)
	}
	for _, addr := range addrs {
		if ip := net.ParseIP(addr); ip != nil && isRegistryBlockedIP(ip) {
			return fmt.Errorf("registry URL resolves to a private or reserved address")
		}
	}
	return nil
}

var registryBlockedNets = func() []*net.IPNet {
	ranges := []string{
		"0.0.0.0/8", "127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
		"169.254.0.0/16", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15",
		"240.0.0.0/4", "255.255.255.255/32",
		"::1/128", "fc00::/7", "fe80::/10", "::/128",
	}
	nets := make([]*net.IPNet, 0, len(ranges))
	for _, r := range ranges {
		if _, n, err := net.ParseCIDR(r); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}()

func isRegistryBlockedIP(ip net.IP) bool {
	for _, n := range registryBlockedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func encodeRepositoryPath(repository string) string {
	parts := strings.Split(strings.Trim(repository, "/"), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func matchesRepositoryScope(repository, scope string) bool {
	repository = strings.Trim(strings.ToLower(repository), "/")
	scope = strings.TrimSpace(scope)
	if scope == "" || scope == "*" {
		return true
	}

	for _, candidate := range strings.Split(scope, ",") {
		candidate = strings.Trim(strings.ToLower(candidate), "/")
		if candidate == "" || candidate == "*" {
			return true
		}
		if repository == candidate || strings.HasPrefix(repository, candidate+"/") {
			return true
		}
	}

	return false
}

func normalizeRepository(repository string) string {
	return strings.ToLower(strings.TrimSpace(repository))
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
