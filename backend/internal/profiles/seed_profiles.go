package profiles

import (
	"strings"
	"time"

	"github.com/babelsuite/babelsuite/internal/suites"
)

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
