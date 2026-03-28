package profiles

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/babelsuite/babelsuite/internal/suites"
)

var (
	ErrSuiteNotFound   = errors.New("suite not found")
	ErrProfileNotFound = errors.New("profile not found")
)

var nonAlphaNumeric = regexp.MustCompile(`[^a-z0-9]+`)

type SecretReference struct {
	Key      string `json:"key" yaml:"key"`
	Provider string `json:"provider" yaml:"provider"`
	Ref      string `json:"ref" yaml:"ref"`
}

type Record struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	FileName    string            `json:"fileName" yaml:"fileName"`
	Description string            `json:"description" yaml:"description"`
	Scope       string            `json:"scope" yaml:"scope"`
	YAML        string            `json:"yaml" yaml:"yaml"`
	SecretRefs  []SecretReference `json:"secretRefs" yaml:"secretRefs"`
	Default     bool              `json:"default" yaml:"default"`
	ExtendsID   string            `json:"extendsId,omitempty" yaml:"extendsId,omitempty"`
	Launchable  bool              `json:"launchable" yaml:"launchable"`
	UpdatedAt   time.Time         `json:"updatedAt" yaml:"updatedAt"`
}

type SuiteSummary struct {
	ID                     string `json:"id"`
	Title                  string `json:"title"`
	Description            string `json:"description"`
	Repository             string `json:"repository"`
	ProfileCount           int    `json:"profileCount"`
	LaunchableCount        int    `json:"launchableCount"`
	DefaultProfileFileName string `json:"defaultProfileFileName"`
}

type SuiteProfiles struct {
	SuiteID                string   `json:"suiteId"`
	SuiteTitle             string   `json:"suiteTitle"`
	SuiteDescription       string   `json:"suiteDescription"`
	Repository             string   `json:"repository"`
	DefaultProfileID       string   `json:"defaultProfileId"`
	DefaultProfileFileName string   `json:"defaultProfileFileName"`
	Profiles               []Record `json:"profiles"`
}

type UpsertRequest struct {
	Name        string            `json:"name"`
	FileName    string            `json:"fileName"`
	Description string            `json:"description"`
	Scope       string            `json:"scope"`
	YAML        string            `json:"yaml"`
	SecretRefs  []SecretReference `json:"secretRefs"`
	Default     bool              `json:"default"`
	ExtendsID   string            `json:"extendsId"`
}

type suiteReader interface {
	List() []suites.Definition
	Get(id string) (*suites.Definition, error)
}

type Document struct {
	Suites map[string]SuiteDocument `json:"suites" yaml:"suites"`
}

type SuiteDocument struct {
	Profiles []Record `json:"profiles" yaml:"profiles"`
}

type Service struct {
	base  suiteReader
	store Store
	mu    sync.Mutex
}

func NewService(base suiteReader, store Store) *Service {
	if base == nil {
		base = suites.NewService()
	}
	if store == nil {
		store = NewMemoryStore()
	}
	return &Service{
		base:  base,
		store: store,
	}
}

func (s *Service) List() []suites.Definition {
	definitions := s.base.List()
	document := s.safeLoadDocument()

	for index := range definitions {
		records := s.mergeSuiteProfiles(definitions[index], document)
		definitions[index].Profiles = toLaunchOptions(records)
	}

	return definitions
}

func (s *Service) Get(id string) (*suites.Definition, error) {
	definition, err := s.base.Get(id)
	if err != nil {
		return nil, err
	}

	document := s.safeLoadDocument()
	definition.Profiles = toLaunchOptions(s.mergeSuiteProfiles(*definition, document))
	return definition, nil
}

func (s *Service) ListSuiteSummaries() ([]SuiteSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	definitions := s.base.List()
	summaries := make([]SuiteSummary, 0, len(definitions))
	for _, definition := range definitions {
		records := s.mergeSuiteProfiles(definition, document)
		_, defaultProfileFileName := defaultProfile(records)
		summaries = append(summaries, SuiteSummary{
			ID:                     definition.ID,
			Title:                  definition.Title,
			Description:            definition.Description,
			Repository:             definition.Repository,
			ProfileCount:           len(records),
			LaunchableCount:        countLaunchable(records),
			DefaultProfileFileName: defaultProfileFileName,
		})
	}

	return summaries, nil
}

func (s *Service) GetSuiteProfiles(suiteID string) (*SuiteProfiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definition, err := s.lookupSuite(suiteID)
	if err != nil {
		return nil, err
	}

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	records := s.mergeSuiteProfiles(*definition, document)
	return buildSuiteProfiles(*definition, records), nil
}

func (s *Service) CreateProfile(suiteID string, request UpsertRequest) (*SuiteProfiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definition, err := s.lookupSuite(suiteID)
	if err != nil {
		return nil, err
	}

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	records := s.mergeSuiteProfiles(*definition, document)
	record := Record{
		ID:         uniqueProfileID(records, request.FileName),
		Launchable: true,
	}
	if err := applyUpsertRequest(&record, request, records, ""); err != nil {
		return nil, err
	}

	records = append(records, record)
	normalizeRecords(records)
	writeSuiteProfiles(document, definition.ID, records)
	if err := s.store.Save(document); err != nil {
		return nil, err
	}

	return buildSuiteProfiles(*definition, records), nil
}

func (s *Service) UpdateProfile(suiteID, profileID string, request UpsertRequest) (*SuiteProfiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definition, err := s.lookupSuite(suiteID)
	if err != nil {
		return nil, err
	}

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	records := s.mergeSuiteProfiles(*definition, document)
	index := indexOfProfile(records, profileID)
	if index == -1 {
		return nil, ErrProfileNotFound
	}

	record := records[index]
	if err := applyUpsertRequest(&record, request, records, record.ID); err != nil {
		return nil, err
	}
	if !record.Launchable {
		record.Default = false
	}
	records[index] = record
	normalizeRecords(records)
	writeSuiteProfiles(document, definition.ID, records)
	if err := s.store.Save(document); err != nil {
		return nil, err
	}

	return buildSuiteProfiles(*definition, records), nil
}

func (s *Service) DeleteProfile(suiteID, profileID string) (*SuiteProfiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definition, err := s.lookupSuite(suiteID)
	if err != nil {
		return nil, err
	}

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	records := s.mergeSuiteProfiles(*definition, document)
	index := indexOfProfile(records, profileID)
	if index == -1 {
		return nil, ErrProfileNotFound
	}
	if !records[index].Launchable {
		return nil, errors.New("Base profiles cannot be deleted.")
	}
	if countLaunchable(records) <= 1 {
		return nil, errors.New("At least one launchable profile must remain.")
	}

	records = append(records[:index], records[index+1:]...)
	normalizeRecords(records)
	writeSuiteProfiles(document, definition.ID, records)
	if err := s.store.Save(document); err != nil {
		return nil, err
	}

	return buildSuiteProfiles(*definition, records), nil
}

func (s *Service) SetDefaultProfile(suiteID, profileID string) (*SuiteProfiles, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	definition, err := s.lookupSuite(suiteID)
	if err != nil {
		return nil, err
	}

	document, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	records := s.mergeSuiteProfiles(*definition, document)
	index := indexOfProfile(records, profileID)
	if index == -1 {
		return nil, ErrProfileNotFound
	}
	if !records[index].Launchable {
		return nil, errors.New("Base profiles cannot be used as the default launch profile.")
	}

	for recordIndex := range records {
		records[recordIndex].Default = records[recordIndex].ID == profileID && records[recordIndex].Launchable
	}
	normalizeRecords(records)
	writeSuiteProfiles(document, definition.ID, records)
	if err := s.store.Save(document); err != nil {
		return nil, err
	}

	return buildSuiteProfiles(*definition, records), nil
}

func (s *Service) lookupSuite(suiteID string) (*suites.Definition, error) {
	definition, err := s.base.Get(strings.TrimSpace(suiteID))
	if err != nil {
		if errors.Is(err, suites.ErrNotFound) {
			return nil, ErrSuiteNotFound
		}
		return nil, err
	}
	return definition, nil
}

func (s *Service) safeLoadDocument() *Document {
	s.mu.Lock()
	defer s.mu.Unlock()

	document, err := s.store.Load()
	if err != nil {
		fallback := defaultDocument()
		return &fallback
	}
	return document
}

func (s *Service) mergeSuiteProfiles(definition suites.Definition, document *Document) []Record {
	seed := seedSuiteProfiles(definition)
	stored := []Record{}
	if document != nil && document.Suites != nil {
		stored = append(stored, document.Suites[definition.ID].Profiles...)
	}

	if len(stored) == 0 {
		return cloneRecords(seed)
	}

	merged := make([]Record, 0, len(seed)+len(stored))
	usedStored := make(map[int]struct{}, len(stored))

	for _, seedRecord := range seed {
		matched := false
		for storedIndex, storedRecord := range stored {
			if _, alreadyUsed := usedStored[storedIndex]; alreadyUsed {
				continue
			}
			if storedRecord.ID == seedRecord.ID || (storedRecord.ID == "" && strings.EqualFold(storedRecord.FileName, seedRecord.FileName)) {
				record := storedRecord
				record.ID = firstNonEmpty(record.ID, seedRecord.ID)
				record.Launchable = seedRecord.Launchable
				record.UpdatedAt = nonZeroTime(record.UpdatedAt, seedRecord.UpdatedAt)
				merged = append(merged, normalizeRecord(record))
				usedStored[storedIndex] = struct{}{}
				matched = true
				break
			}
		}
		if !matched {
			merged = append(merged, normalizeRecord(seedRecord))
		}
	}

	for storedIndex, storedRecord := range stored {
		if _, alreadyUsed := usedStored[storedIndex]; alreadyUsed {
			continue
		}
		merged = append(merged, normalizeRecord(storedRecord))
	}

	normalizeRecords(merged)
	return merged
}

func buildSuiteProfiles(definition suites.Definition, records []Record) *SuiteProfiles {
	cloned := cloneRecords(records)
	defaultProfileID, defaultProfileFileName := defaultProfile(cloned)
	return &SuiteProfiles{
		SuiteID:                definition.ID,
		SuiteTitle:             definition.Title,
		SuiteDescription:       definition.Description,
		Repository:             definition.Repository,
		DefaultProfileID:       defaultProfileID,
		DefaultProfileFileName: defaultProfileFileName,
		Profiles:               cloned,
	}
}

func writeSuiteProfiles(document *Document, suiteID string, records []Record) {
	if document.Suites == nil {
		document.Suites = map[string]SuiteDocument{}
	}
	document.Suites[suiteID] = SuiteDocument{
		Profiles: cloneRecords(records),
	}
}

func applyUpsertRequest(record *Record, request UpsertRequest, current []Record, currentID string) error {
	record.Name = strings.TrimSpace(request.Name)
	record.FileName = strings.TrimSpace(request.FileName)
	record.Description = strings.TrimSpace(request.Description)
	record.Scope = firstNonEmpty(strings.TrimSpace(request.Scope), scopeFromFileName(request.FileName))
	record.YAML = strings.TrimSpace(request.YAML)
	record.SecretRefs = compactSecretRefs(request.SecretRefs)
	record.Default = request.Default && record.Launchable
	record.ExtendsID = strings.TrimSpace(request.ExtendsID)
	record.UpdatedAt = time.Now().UTC()

	if record.ExtendsID == "" && record.Launchable {
		record.ExtendsID = "base"
	}
	if !record.Launchable {
		record.Default = false
	}

	return validateRecord(*record, current, currentID)
}

func validateRecord(record Record, current []Record, currentID string) error {
	if record.Name == "" {
		return errors.New("Profile name is required.")
	}
	if record.FileName == "" {
		return errors.New("Profile file name is required.")
	}
	if !strings.HasSuffix(strings.ToLower(record.FileName), ".yaml") && !strings.HasSuffix(strings.ToLower(record.FileName), ".yml") {
		return errors.New("Profile file names must end with .yaml or .yml.")
	}
	if record.YAML == "" {
		return errors.New("Profile YAML is required.")
	}

	var parsed any
	if err := yaml.Unmarshal([]byte(record.YAML), &parsed); err != nil {
		return fmt.Errorf("Invalid YAML: %v", err)
	}

	for _, existing := range current {
		if existing.ID == currentID {
			continue
		}
		if strings.EqualFold(existing.FileName, record.FileName) {
			return errors.New("A profile with that file name already exists for this suite.")
		}
	}

	if record.ExtendsID != "" && indexOfProfile(current, record.ExtendsID) == -1 && record.ExtendsID != currentID {
		return errors.New("The selected base profile was not found.")
	}

	for _, secretRef := range record.SecretRefs {
		if secretRef.Key == "" || secretRef.Provider == "" || secretRef.Ref == "" {
			return errors.New("Secret references need a key, provider, and reference.")
		}
	}

	return nil
}

func normalizeDocument(document *Document) {
	if document.Suites == nil {
		document.Suites = map[string]SuiteDocument{}
	}
	for suiteID, suiteDocument := range document.Suites {
		normalizeRecords(suiteDocument.Profiles)
		document.Suites[strings.TrimSpace(suiteID)] = SuiteDocument{
			Profiles: suiteDocument.Profiles,
		}
	}
}

func normalizeRecords(records []Record) {
	defaultSeen := false
	for index := range records {
		records[index] = normalizeRecord(records[index])
		if !records[index].Launchable {
			records[index].Default = false
			continue
		}
		if records[index].Default && !defaultSeen {
			defaultSeen = true
			continue
		}
		if records[index].Default && defaultSeen {
			records[index].Default = false
		}
	}

	if defaultSeen {
		return
	}

	for index := range records {
		if records[index].Launchable {
			records[index].Default = true
			return
		}
	}
}

func normalizeRecord(record Record) Record {
	record.ID = strings.TrimSpace(record.ID)
	record.Name = strings.TrimSpace(record.Name)
	record.FileName = strings.TrimSpace(record.FileName)
	record.Description = strings.TrimSpace(record.Description)
	record.Scope = firstNonEmpty(strings.TrimSpace(record.Scope), scopeFromFileName(record.FileName))
	record.YAML = strings.TrimSpace(record.YAML)
	record.ExtendsID = strings.TrimSpace(record.ExtendsID)
	record.SecretRefs = compactSecretRefs(record.SecretRefs)
	record.UpdatedAt = nonZeroTime(record.UpdatedAt, time.Now().UTC())
	return record
}

func seedSuiteProfiles(definition suites.Definition) []Record {
	records := []Record{
		{
			ID:          "base",
			Name:        "Base Runtime",
			FileName:    "base.yaml",
			Description: "Canonical suite defaults shared by every execution context before local, CI, or staging overrides are layered in.",
			Scope:       "Base",
			YAML:        baseProfileYAML(definition.ID),
			SecretRefs:  baseProfileSecrets(definition.ID),
			Default:     false,
			Launchable:  false,
			UpdatedAt:   time.Now().UTC(),
		},
	}

	for _, profile := range definition.Profiles {
		records = append(records, Record{
			ID:          profileIDFromFileName(profile.FileName),
			Name:        firstNonEmpty(strings.TrimSpace(profile.Label), labelFromFileName(profile.FileName)),
			FileName:    profile.FileName,
			Description: strings.TrimSpace(profile.Description),
			Scope:       scopeFromFileName(profile.FileName),
			YAML:        seedProfileYAML(definition.ID, profile.FileName),
			SecretRefs:  seedProfileSecrets(definition.ID, profile.FileName),
			Default:     profile.Default,
			ExtendsID:   "base",
			Launchable:  true,
			UpdatedAt:   time.Now().UTC(),
		})
	}

	normalizeRecords(records)
	return records
}

func baseProfileYAML(suiteID string) string {
	switch suiteID {
	case "payment-suite":
		return "env:\n  LOG_LEVEL: info\n  PAYMENTS_API_BASE_URL: http://payment-gateway.internal\n  FRAUD_STRATEGY: standard\nservices:\n  postgresURL: postgres://test:test@db:5432/payments\n  kafkaBroker: kafka:9092\n"
	case "fleet-control-room":
		return "env:\n  LOG_LEVEL: info\n  TELEMETRY_PROFILE: balanced\n  DISPATCHER_BASE_URL: http://dispatcher-api.internal\nservices:\n  redisAddress: redis-cache:6379\n  plannerRefreshInterval: 5s\n"
	case "storefront-browser-lab":
		return "env:\n  LOG_LEVEL: info\n  STOREFRONT_BASE_URL: http://storefront-ui.internal\n  PLAYWRIGHT_BROWSER: chromium\nservices:\n  kafkaBroker: kafka:9092\n  mockApiBaseUrl: http://storefront-api.internal\n"
	case "identity-broker":
		return "env:\n  LOG_LEVEL: info\n  SESSION_STORE: postgres\n  OIDC_DISCOVERY_URL: http://oidc-mock.internal/.well-known/openid-configuration\nservices:\n  postgresURL: postgres://test:test@broker-db:5432/identity\n  sessionQueue: session-worker\n"
	default:
		return "env:\n  LOG_LEVEL: info\nservices:\n  endpoint: http://service.internal\n"
	}
}

func baseProfileSecrets(suiteID string) []SecretReference {
	switch suiteID {
	case "payment-suite":
		return []SecretReference{
			{Key: "JWT_PRIVATE_KEY", Provider: "Vault", Ref: "kv/payment-suite/jwt-private-key"},
		}
	case "fleet-control-room":
		return []SecretReference{
			{Key: "MAPBOX_TOKEN", Provider: "Vault", Ref: "kv/fleet-control-room/mapbox-token"},
		}
	case "storefront-browser-lab":
		return []SecretReference{
			{Key: "PLAYWRIGHT_STORAGE_STATE", Provider: "Vault", Ref: "kv/storefront-browser-lab/playwright-storage-state"},
		}
	case "identity-broker":
		return []SecretReference{
			{Key: "BROKER_SIGNING_KEY", Provider: "Vault", Ref: "kv/identity-broker/signing-key"},
		}
	default:
		return nil
	}
}

func seedProfileYAML(suiteID, fileName string) string {
	switch suiteID {
	case "payment-suite":
		switch fileName {
		case "local.yaml":
			return "env:\n  LOG_LEVEL: debug\n  FRAUD_STRATEGY: permissive\nservices:\n  apiPort: 18080\n  workerReplicaCount: 1\n"
		case "staging.yaml":
			return "env:\n  PAYMENTS_API_BASE_URL: https://payments.staging.company.test\n  FRAUD_STRATEGY: strict\nservices:\n  workerReplicaCount: 3\n  apiPort: 8080\n"
		case "year.yaml":
			return "env:\n  LEDGER_PERIOD: year_end\n  FRAUD_STRATEGY: settlement_review\nservices:\n  settlementBatchSize: 500\n  replayHistoricalCharges: true\n"
		}
	case "fleet-control-room":
		switch fileName {
		case "local.yaml":
			return "env:\n  LOG_LEVEL: debug\n  TELEMETRY_PROFILE: verbose\nservices:\n  uiPort: 13000\n  dispatcherPort: 18081\n"
		case "perf.yaml":
			return "env:\n  TELEMETRY_PROFILE: burst\n  ENABLE_ROUTE_HEATMAP: true\nservices:\n  plannerWorkers: 6\n  ingestBatchSize: 500\n"
		case "staging.yaml":
			return "env:\n  DISPATCHER_BASE_URL: https://dispatcher.staging.company.test\n  TELEMETRY_PROFILE: realistic\nservices:\n  plannerWorkers: 3\n  uiPort: 8080\n"
		}
	case "storefront-browser-lab":
		switch fileName {
		case "local.yaml":
			return "env:\n  LOG_LEVEL: debug\n  PLAYWRIGHT_TRACE: on\n  MOCK_SCENARIO: local\nservices:\n  uiPort: 14010\n  consumerWorkers: 1\n"
		case "ci.yaml":
			return "env:\n  PLAYWRIGHT_HEADLESS: true\n  PLAYWRIGHT_TRACE: retain-on-failure\n  MOCK_SCENARIO: ci\nservices:\n  uiPort: 8080\n  consumerWorkers: 2\n"
		case "promo.yaml":
			return "env:\n  CAMPAIGN_MODE: spring_promo\n  PLAYWRIGHT_HEADLESS: true\n  MOCK_SCENARIO: promo\nservices:\n  consumerWorkers: 4\n  productSeedSet: promo-heavy\n"
		}
	case "identity-broker":
		switch fileName {
		case "local.yaml":
			return "env:\n  LOG_LEVEL: debug\n  COOKIE_SECURE: false\nservices:\n  brokerPort: 18082\n  sessionWorkerConcurrency: 1\n"
		case "canary.yaml":
			return "env:\n  SESSION_STORE: redis\n  ENABLE_NEW_COOKIE_POLICY: true\nservices:\n  sessionWorkerConcurrency: 4\n  tokenRotationWindow: 15m\n"
		case "ci.yaml":
			return "env:\n  LOG_LEVEL: warn\n  ENABLE_NEW_COOKIE_POLICY: true\nservices:\n  brokerPort: 8080\n  sessionWorkerConcurrency: 2\n"
		}
	}

	return "env:\n  LOG_LEVEL: info\n"
}

func seedProfileSecrets(suiteID, fileName string) []SecretReference {
	switch suiteID {
	case "payment-suite":
		switch fileName {
		case "local.yaml":
			return []SecretReference{
				{Key: "STRIPE_API_KEY", Provider: "Local Secret", Ref: "secrets://developer/stripe-api-key"},
			}
		case "staging.yaml":
			return []SecretReference{
				{Key: "DB_PASSWORD", Provider: "Vault", Ref: "kv/payment-suite/staging-db-password"},
			}
		case "year.yaml":
			return []SecretReference{
				{Key: "SETTLEMENT_SFTP_KEY", Provider: "Vault", Ref: "kv/payment-suite/year-end-sftp-key"},
			}
		}
	case "fleet-control-room":
		switch fileName {
		case "local.yaml":
			return []SecretReference{
				{Key: "TELEMETRY_TOKEN", Provider: "Local Secret", Ref: "secrets://developer/telemetry-token"},
			}
		case "staging.yaml":
			return []SecretReference{
				{Key: "ROUTING_API_KEY", Provider: "Vault", Ref: "kv/fleet-control-room/staging-routing-key"},
			}
		}
	case "storefront-browser-lab":
		switch fileName {
		case "local.yaml":
			return []SecretReference{
				{Key: "SHOPPER_SESSION_COOKIE", Provider: "Local Secret", Ref: "secrets://developer/shopper-session-cookie"},
			}
		case "ci.yaml":
			return []SecretReference{
				{Key: "PLAYWRIGHT_RECORD_KEY", Provider: "Vault", Ref: "kv/storefront-browser-lab/ci-record-key"},
			}
		case "promo.yaml":
			return []SecretReference{
				{Key: "CAMPAIGN_SIGNING_KEY", Provider: "Vault", Ref: "kv/storefront-browser-lab/promo-signing-key"},
			}
		}
	case "identity-broker":
		switch fileName {
		case "local.yaml":
			return []SecretReference{
				{Key: "OIDC_CLIENT_SECRET", Provider: "Local Secret", Ref: "secrets://developer/oidc-client-secret"},
			}
		case "canary.yaml":
			return []SecretReference{
				{Key: "SESSION_REDIS_PASSWORD", Provider: "Vault", Ref: "kv/identity-broker/canary-redis-password"},
			}
		case "ci.yaml":
			return []SecretReference{
				{Key: "BROKER_TEST_CERT", Provider: "Vault", Ref: "kv/identity-broker/ci-test-cert"},
			}
		}
	}
	return nil
}

func toLaunchOptions(records []Record) []suites.ProfileOption {
	options := make([]suites.ProfileOption, 0, len(records))
	for _, record := range records {
		if !record.Launchable {
			continue
		}
		options = append(options, suites.ProfileOption{
			FileName:    record.FileName,
			Label:       record.Name,
			Description: record.Description,
			Default:     record.Default,
		})
	}
	return options
}

func cloneDocument(document Document) Document {
	clone := defaultDocument()
	for suiteID, suiteDocument := range document.Suites {
		clone.Suites[suiteID] = SuiteDocument{
			Profiles: cloneRecords(suiteDocument.Profiles),
		}
	}
	return clone
}

func cloneRecords(records []Record) []Record {
	clone := make([]Record, len(records))
	for index, record := range records {
		clone[index] = record
		clone[index].SecretRefs = append([]SecretReference{}, record.SecretRefs...)
	}
	return clone
}

func compactSecretRefs(secretRefs []SecretReference) []SecretReference {
	result := make([]SecretReference, 0, len(secretRefs))
	for _, secretRef := range secretRefs {
		key := strings.TrimSpace(secretRef.Key)
		provider := strings.TrimSpace(secretRef.Provider)
		ref := strings.TrimSpace(secretRef.Ref)
		if key == "" && provider == "" && ref == "" {
			continue
		}
		result = append(result, SecretReference{
			Key:      key,
			Provider: provider,
			Ref:      ref,
		})
	}
	return result
}

func defaultDocument() Document {
	return Document{
		Suites: map[string]SuiteDocument{},
	}
}

func defaultProfile(records []Record) (string, string) {
	for _, record := range records {
		if record.Launchable && record.Default {
			return record.ID, record.FileName
		}
	}
	return "", ""
}

func countLaunchable(records []Record) int {
	count := 0
	for _, record := range records {
		if record.Launchable {
			count++
		}
	}
	return count
}

func uniqueProfileID(records []Record, fileName string) string {
	baseID := profileIDFromFileName(fileName)
	candidate := baseID
	index := 2
	for indexOfProfile(records, candidate) != -1 {
		candidate = fmt.Sprintf("%s-%d", baseID, index)
		index++
	}
	if candidate == "" {
		return "profile-" + uuid.NewString()[:8]
	}
	return candidate
}

func profileIDFromFileName(fileName string) string {
	trimmed := strings.TrimSpace(strings.ToLower(fileName))
	trimmed = strings.TrimSuffix(trimmed, ".yaml")
	trimmed = strings.TrimSuffix(trimmed, ".yml")
	trimmed = nonAlphaNumeric.ReplaceAllString(trimmed, "-")
	return strings.Trim(trimmed, "-")
}

func scopeFromFileName(fileName string) string {
	normalized := strings.ToLower(strings.TrimSpace(fileName))
	switch {
	case strings.Contains(normalized, "base"):
		return "Base"
	case strings.Contains(normalized, "local"):
		return "Local"
	case strings.Contains(normalized, "ci"):
		return "CI"
	case strings.Contains(normalized, "perf"):
		return "Performance"
	case strings.Contains(normalized, "stage"):
		return "Staging"
	case strings.Contains(normalized, "canary"):
		return "Canary"
	case strings.Contains(normalized, "year"):
		return "Year End"
	default:
		return labelFromFileName(fileName)
	}
}

func labelFromFileName(fileName string) string {
	trimmed := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(fileName), ".yaml"), ".yml")
	parts := strings.FieldsFunc(trimmed, func(value rune) bool {
		return value == '-' || value == '_' || value == '.'
	})
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func indexOfProfile(records []Record, profileID string) int {
	for index, record := range records {
		if record.ID == strings.TrimSpace(profileID) {
			return index
		}
	}
	return -1
}

func nonZeroTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
