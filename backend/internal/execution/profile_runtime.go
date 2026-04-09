package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/platform"
	"github.com/babelsuite/babelsuite/internal/profiles"
	"gopkg.in/yaml.v3"
)

var errProfileSecretUnavailable = errors.New("profile secret unavailable")

type suiteProfilesReader interface {
	GetSuiteProfiles(suiteID string) (*profiles.SuiteProfiles, error)
}

type profileRuntimeDocument struct {
	Env        map[string]string
	Services   map[string]map[string]string
	SecretRefs []profiles.SecretReference
}

func (s *Service) resolveExecutionRuntimeOverlay(ctx context.Context, suiteID, profile string) (executionRuntimeOverlay, error) {
	settings, err := s.loadPlatformSettings()
	if err != nil {
		return executionRuntimeOverlay{}, fmt.Errorf("%w: could not load platform settings: %v", ErrProfileRuntime, err)
	}

	profileRuntime, err := s.resolveManagedProfileRuntime(suiteID, profile)
	if err != nil {
		return executionRuntimeOverlay{}, err
	}

	overlay := executionRuntimeOverlay{
		Env:      mergeRuntimeMaps(profileRuntime.Env, platformGlobalOverrideEnv(settings)),
		Services: cloneServiceEnvMap(profileRuntime.Services),
	}

	if len(profileRuntime.SecretRefs) > 0 {
		secretEnv, err := resolveProfileSecrets(ctx, settings, profileRuntime.SecretRefs)
		if err != nil {
			return executionRuntimeOverlay{}, err
		}
		overlay.SecretEnv = secretEnv
	}

	return overlay, nil
}

func (s *Service) resolveNodeRuntimeEnv(executionID string, node topologyNode) map[string]string {
	s.mu.Lock()
	item := s.executions[executionID]
	s.mu.Unlock()
	if item == nil {
		return cloneRuntimeMap(node.RuntimeEnv)
	}

	serviceEnv := item.runtime.serviceEnv(node)
	return mergeRuntimeMaps(
		item.runtime.Env,
		node.RuntimeEnv,
		serviceEnv,
		item.runtime.SecretEnv,
	)
}

func (o executionRuntimeOverlay) serviceEnv(node topologyNode) map[string]string {
	if len(o.Services) == 0 {
		return nil
	}

	if env := o.Services[strings.TrimSpace(node.ID)]; len(env) > 0 {
		return cloneRuntimeMap(env)
	}
	if env := o.Services[strings.TrimSpace(node.Name)]; len(env) > 0 {
		return cloneRuntimeMap(env)
	}
	return nil
}

func (s *Service) resolveManagedProfileRuntime(suiteID, profile string) (struct {
	Env        map[string]string
	Services   map[string]map[string]string
	SecretRefs []profiles.SecretReference
}, error) {
	reader, ok := s.suiteSource.(suiteProfilesReader)
	if !ok {
		return struct {
			Env        map[string]string
			Services   map[string]map[string]string
			SecretRefs []profiles.SecretReference
		}{}, nil
	}

	payload, err := reader.GetSuiteProfiles(suiteID)
	if err != nil {
		return struct {
			Env        map[string]string
			Services   map[string]map[string]string
			SecretRefs []profiles.SecretReference
		}{}, fmt.Errorf("%w: could not load managed profiles for suite %q: %v", ErrProfileRuntime, suiteID, err)
	}

	record := findProfileRecord(payload.Profiles, profile)
	if record == nil {
		return struct {
			Env        map[string]string
			Services   map[string]map[string]string
			SecretRefs []profiles.SecretReference
		}{}, nil
	}

	chain, err := resolveProfileChain(payload.Profiles, record.ID)
	if err != nil {
		return struct {
			Env        map[string]string
			Services   map[string]map[string]string
			SecretRefs []profiles.SecretReference
		}{}, fmt.Errorf("%w: %v", ErrProfileRuntime, err)
	}

	runtime := struct {
		Env        map[string]string
		Services   map[string]map[string]string
		SecretRefs []profiles.SecretReference
	}{
		Services: map[string]map[string]string{},
	}

	for _, current := range chain {
		parsed, err := parseManagedProfileYAML(current.YAML)
		if err != nil {
			return struct {
				Env        map[string]string
				Services   map[string]map[string]string
				SecretRefs []profiles.SecretReference
			}{}, fmt.Errorf("%w: could not parse %s: %v", ErrProfileRuntime, current.FileName, err)
		}
		runtime.Env = mergeRuntimeMaps(runtime.Env, parsed.Env)
		runtime.Services = mergeServiceEnvMaps(runtime.Services, parsed.Services)
		runtime.SecretRefs = mergeSecretRefs(runtime.SecretRefs, parsed.SecretRefs, current.SecretRefs)
	}

	return runtime, nil
}

func parseManagedProfileYAML(source string) (profileRuntimeDocument, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return profileRuntimeDocument{}, nil
	}

	var document map[string]any
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		return profileRuntimeDocument{}, err
	}

	runtime := profileRuntimeDocument{
		Env:        scalarStringMap(document["env"]),
		Services:   map[string]map[string]string{},
		SecretRefs: profiles.ExtractSecretRefsFromYAML(source),
	}

	if services, ok := document["services"].(map[string]any); ok {
		for name, rawService := range services {
			serviceMap, ok := rawService.(map[string]any)
			if !ok {
				continue
			}
			env := scalarStringMap(serviceMap["env"])
			if len(env) == 0 {
				continue
			}
			runtime.Services[strings.TrimSpace(name)] = env
		}
	}

	return runtime, nil
}

func resolveProfileChain(records []profiles.Record, selectedID string) ([]profiles.Record, error) {
	byID := make(map[string]profiles.Record, len(records))
	for _, record := range records {
		byID[strings.TrimSpace(record.ID)] = record
	}

	ordered := make([]profiles.Record, 0, len(records))
	visiting := map[string]bool{}
	visited := map[string]bool{}

	var visit func(string) error
	visit = func(id string) error {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("profile inheritance cycle detected at %q", id)
		}
		if visited[id] {
			return nil
		}

		record, ok := byID[id]
		if !ok {
			return fmt.Errorf("base profile %q was not found", id)
		}

		visiting[id] = true
		if err := visit(record.ExtendsID); err != nil {
			return err
		}
		visiting[id] = false
		visited[id] = true
		ordered = append(ordered, record)
		return nil
	}

	if err := visit(selectedID); err != nil {
		return nil, err
	}

	return ordered, nil
}

func findProfileRecord(records []profiles.Record, fileName string) *profiles.Record {
	for index := range records {
		if strings.EqualFold(strings.TrimSpace(records[index].FileName), strings.TrimSpace(fileName)) {
			return &records[index]
		}
	}
	return nil
}

func platformGlobalOverrideEnv(settings *platform.PlatformSettings) map[string]string {
	if settings == nil || len(settings.Secrets.GlobalOverrides) == 0 {
		return nil
	}

	env := make(map[string]string, len(settings.Secrets.GlobalOverrides))
	for _, override := range settings.Secrets.GlobalOverrides {
		key := strings.TrimSpace(override.Key)
		if key == "" {
			continue
		}
		env[key] = override.Value
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

func mergeServiceEnvMaps(base map[string]map[string]string, overlays ...map[string]map[string]string) map[string]map[string]string {
	size := len(base)
	for _, overlay := range overlays {
		size += len(overlay)
	}
	if size == 0 {
		return nil
	}

	result := make(map[string]map[string]string, size)
	for name, env := range base {
		result[name] = cloneRuntimeMap(env)
	}
	for _, overlay := range overlays {
		for name, env := range overlay {
			result[name] = mergeRuntimeMaps(result[name], env)
		}
	}
	return result
}

func cloneServiceEnvMap(input map[string]map[string]string) map[string]map[string]string {
	if len(input) == 0 {
		return nil
	}

	output := make(map[string]map[string]string, len(input))
	for name, env := range input {
		output[name] = cloneRuntimeMap(env)
	}
	return output
}

func mergeRuntimeMaps(items ...map[string]string) map[string]string {
	size := 0
	for _, item := range items {
		size += len(item)
	}
	if size == 0 {
		return nil
	}

	merged := make(map[string]string, size)
	for _, item := range items {
		for key, value := range item {
			merged[key] = value
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func mergeSecretRefs(base []profiles.SecretReference, overlays ...[]profiles.SecretReference) []profiles.SecretReference {
	total := len(base)
	for _, overlay := range overlays {
		total += len(overlay)
	}
	if total == 0 {
		return nil
	}

	ordered := make([]profiles.SecretReference, 0, total)
	indexByKey := make(map[string]int, total)
	appendRef := func(ref profiles.SecretReference) {
		key := strings.TrimSpace(ref.Key)
		if key == "" {
			return
		}
		if index, ok := indexByKey[key]; ok {
			ordered[index] = ref
			return
		}
		indexByKey[key] = len(ordered)
		ordered = append(ordered, ref)
	}

	for _, ref := range base {
		appendRef(ref)
	}
	for _, overlay := range overlays {
		for _, ref := range overlay {
			appendRef(ref)
		}
	}
	return ordered
}

func scalarStringMap(value any) map[string]string {
	items, ok := value.(map[string]any)
	if !ok || len(items) == 0 {
		return nil
	}

	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make(map[string]string, len(items))
	for _, key := range keys {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		switch typed := items[key].(type) {
		case nil:
			continue
		case string:
			result[trimmedKey] = typed
		default:
			result[trimmedKey] = fmt.Sprint(typed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func resolveProfileSecrets(ctx context.Context, settings *platform.PlatformSettings, refs []profiles.SecretReference) (map[string]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	resolver := platformSecretResolver{
		settings: settings,
		client:   &http.Client{Timeout: 5 * time.Second},
	}

	env := make(map[string]string, len(refs))
	for _, ref := range refs {
		key := strings.TrimSpace(ref.Key)
		if key == "" {
			continue
		}

		value, err := resolver.Resolve(ctx, ref)
		if err != nil {
			if errors.Is(err, errProfileSecretUnavailable) {
				continue
			}
			return nil, err
		}
		env[key] = value
	}
	if len(env) == 0 {
		return nil, nil
	}
	return env, nil
}

type platformSecretResolver struct {
	settings *platform.PlatformSettings
	client   *http.Client
}

func (r platformSecretResolver) Resolve(ctx context.Context, ref profiles.SecretReference) (string, error) {
	switch normalizeSecretProvider(ref.Provider, ref.Ref, r.settings) {
	case "vault":
		return r.resolveVault(ctx, ref)
	case "local":
		return r.resolveLocal(ref)
	default:
		return "", fmt.Errorf("%w: unsupported profile secret provider %q for %s", ErrProfileRuntime, ref.Provider, ref.Key)
	}
}

func (r platformSecretResolver) resolveLocal(ref profiles.SecretReference) (string, error) {
	key := strings.TrimSpace(ref.Key)
	if r.settings != nil {
		for _, override := range r.settings.Secrets.GlobalOverrides {
			if strings.EqualFold(strings.TrimSpace(override.Key), key) {
				return override.Value, nil
			}
		}
	}

	for _, candidate := range localSecretEnvCandidates(ref) {
		if value, ok := os.LookupEnv(candidate); ok {
			return value, nil
		}
	}

	return "", fmt.Errorf("%w: could not resolve local secret %q", ErrProfileRuntime, ref.Ref)
}

func (r platformSecretResolver) resolveVault(ctx context.Context, ref profiles.SecretReference) (string, error) {
	if vaultUnavailable(r.settings) {
		return "", fmt.Errorf("%w: Vault is not configured", errProfileSecretUnavailable)
	}

	if provider := strings.ToLower(strings.TrimSpace(r.settings.Secrets.Provider)); provider != "" && provider != "vault" && provider != "none" {
		return "", fmt.Errorf("%w: platform secrets provider %q cannot resolve Vault references", ErrProfileRuntime, r.settings.Secrets.Provider)
	}

	address := strings.TrimSpace(r.settings.Secrets.VaultAddress)
	if address == "" {
		return "", fmt.Errorf("%w: Vault address is not configured", ErrProfileRuntime)
	}

	token, ok := firstEnvValue("BABELSUITE_VAULT_TOKEN", "VAULT_TOKEN")
	if !ok {
		return "", fmt.Errorf("%w: VAULT_TOKEN or BABELSUITE_VAULT_TOKEN is required to resolve Vault secret %q", ErrProfileRuntime, ref.Key)
	}

	secretPath, field, err := normalizeVaultRef(ref.Ref, r.settings.Secrets.SecretPrefix)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrProfileRuntime, err)
	}

	target, err := url.JoinPath(strings.TrimRight(address, "/"), "v1", secretPath)
	if err != nil {
		return "", fmt.Errorf("%w: invalid Vault address %q", ErrProfileRuntime, address)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("%w: could not build Vault request", ErrProfileRuntime)
	}
	req.Header.Set("X-Vault-Token", token)
	if namespace := strings.TrimSpace(r.settings.Secrets.VaultNamespace); namespace != "" {
		req.Header.Set("X-Vault-Namespace", namespace)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: Vault lookup failed for %q: %v", ErrProfileRuntime, ref.Key, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = resp.Status
		}
		return "", fmt.Errorf("%w: Vault lookup failed for %q: %s", ErrProfileRuntime, ref.Key, detail)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("%w: could not decode Vault response for %q", ErrProfileRuntime, ref.Key)
	}

	value, err := extractVaultSecretValue(payload, ref.Key, field)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrProfileRuntime, err)
	}
	return value, nil
}

func vaultUnavailable(settings *platform.PlatformSettings) bool {
	if settings == nil {
		return true
	}

	provider := strings.ToLower(strings.TrimSpace(settings.Secrets.Provider))
	switch provider {
	case "none":
		return true
	case "":
		return strings.TrimSpace(settings.Secrets.VaultAddress) == "" &&
			strings.TrimSpace(settings.Secrets.VaultNamespace) == "" &&
			strings.TrimSpace(settings.Secrets.VaultRole) == "" &&
			strings.TrimSpace(settings.Secrets.SecretPrefix) == ""
	default:
		return false
	}
}

func normalizeSecretProvider(provider, ref string, settings *platform.PlatformSettings) string {
	lowerProvider := strings.ToLower(strings.TrimSpace(provider))
	switch {
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref)), "vault://"):
		return "vault"
	case strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref)), "secrets://"):
		return "local"
	case strings.Contains(lowerProvider, "vault"):
		return "vault"
	case strings.Contains(lowerProvider, "local"):
		return "local"
	case lowerProvider == "" && settings != nil && strings.EqualFold(strings.TrimSpace(settings.Secrets.Provider), "vault"):
		return "vault"
	default:
		return lowerProvider
	}
}

func normalizeVaultRef(rawRef, prefix string) (string, string, error) {
	ref := strings.TrimSpace(rawRef)
	if ref == "" {
		return "", "", fmt.Errorf("Vault secret reference is empty")
	}

	ref = strings.TrimPrefix(ref, "vault://")
	pathPart := ref
	field := ""
	if hash := strings.Index(pathPart, "#"); hash >= 0 {
		field = strings.TrimSpace(pathPart[hash+1:])
		pathPart = pathPart[:hash]
	}

	pathPart = strings.Trim(strings.TrimSpace(pathPart), "/")
	if pathPart == "" {
		return "", "", fmt.Errorf("Vault secret reference %q is missing a path", rawRef)
	}

	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix != "" && !strings.HasPrefix(pathPart, prefix) {
		mount := prefix
		if slash := strings.Index(mount, "/"); slash >= 0 {
			mount = mount[:slash]
		}
		if mount == "" || !strings.HasPrefix(pathPart, mount+"/") {
			pathPart = prefix + "/" + pathPart
		}
	}

	return path.Clean(pathPart), field, nil
}

func extractVaultSecretValue(payload map[string]any, envKey, explicitField string) (string, error) {
	data, ok := payload["data"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("Vault response is missing data for %q", envKey)
	}

	if nested, ok := data["data"].(map[string]any); ok {
		return selectSecretField(nested, envKey, explicitField)
	}
	return selectSecretField(data, envKey, explicitField)
}

func selectSecretField(values map[string]any, envKey, explicitField string) (string, error) {
	if len(values) == 0 {
		return "", fmt.Errorf("secret payload for %q is empty", envKey)
	}

	candidates := make([]string, 0, 8)
	addCandidate := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		for _, existing := range candidates {
			if existing == candidate {
				return
			}
		}
		candidates = append(candidates, candidate)
	}

	addCandidate(explicitField)
	addCandidate(envKey)
	addCandidate(strings.ToLower(envKey))
	addCandidate(strings.ToLower(strings.ReplaceAll(envKey, "_", "-")))
	addCandidate(strings.ToLower(strings.ReplaceAll(envKey, "-", "_")))
	addCandidate("value")
	addCandidate("secret")
	addCandidate("token")

	for _, candidate := range candidates {
		if value, ok := lookupScalarValue(values, candidate); ok {
			return value, nil
		}
	}

	scalarValues := make([]string, 0, len(values))
	for _, raw := range values {
		if value, ok := scalarString(raw); ok {
			scalarValues = append(scalarValues, value)
		}
	}
	if len(scalarValues) == 1 {
		return scalarValues[0], nil
	}

	return "", fmt.Errorf("could not choose a field for secret %q", envKey)
}

func lookupScalarValue(values map[string]any, candidate string) (string, bool) {
	if raw, ok := values[candidate]; ok {
		return scalarString(raw)
	}
	for key, raw := range values {
		if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(candidate)) {
			return scalarString(raw)
		}
	}
	return "", false
}

func scalarString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case fmt.Stringer:
		return typed.String(), true
	case bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(typed), true
	default:
		return "", false
	}
}

func localSecretEnvCandidates(ref profiles.SecretReference) []string {
	pathRef := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ref.Ref), "secrets://"))
	candidates := []string{
		strings.TrimSpace(ref.Key),
		sanitizeEnvToken(pathRef),
		"BABELSUITE_SECRET_" + sanitizeEnvToken(pathRef),
	}

	if slash := strings.LastIndex(pathRef, "/"); slash >= 0 && slash+1 < len(pathRef) {
		candidates = append(candidates, sanitizeEnvToken(pathRef[slash+1:]))
	}

	ordered := make([]string, 0, len(candidates))
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		candidate = strings.Trim(candidate, "_")
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		ordered = append(ordered, candidate)
	}
	return ordered
}

func sanitizeEnvToken(value string) string {
	replacer := strings.NewReplacer("/", "_", "-", "_", ".", "_", ":", "_")
	value = replacer.Replace(strings.TrimSpace(value))
	return strings.ToUpper(value)
}

func firstEnvValue(keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			return value, true
		}
	}
	return "", false
}
