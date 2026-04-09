Storefront Browser Lab

Browser-first commerce lab with Kafka event streams, Playwright checkout journeys, and mock APIs for promo traffic validation.

Structure

- `suite.star`: declarative topology
- `profiles/`: Browser, Kafka, and mock dispatch settings for local, CI, and campaign traffic.
- `api/`: Order and catalog contracts exposed to the UI and background consumer.
- `mock/`: Mock API payloads for product catalog and order submission paths.
- `tasks/`: Kafka bootstrap and browser fixture warm-up hooks.
- `tests/`: Playwright coverage for checkout success and cart abandonment journeys.
- `fixtures/`: Seeded products, campaigns, and browser-side user sessions.
- `policies/`: Event schema and checkout latency validation rules.
