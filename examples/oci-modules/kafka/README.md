@babelsuite/kafka

Pure Starlark Kafka module built on top of BabelSuite's built-in runtime containers.

Details

- Repository: `localhost:5000/babelsuite/kafka`
- Version: `1.2.3`
- Tags: `1.2.3`, `1.2.2`, `latest`
- Pull: `babelctl run localhost:5000/babelsuite/kafka:1.2.3`
- Fork: `babelctl fork localhost:5000/babelsuite/kafka:1.2.3 ./stdlib-kafka`
- Entrypoint: `module.star`
- Helpers: `kafka`, `create_topic`, `delete_topic`, `set_group_offset`, `disconnect`

Usage

See `module.star` for the module implementation and `usage.star` for a consumer example.
