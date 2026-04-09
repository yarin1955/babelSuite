load("@babelsuite/runtime", "container")
load("_shared.star", "merge_dicts")

def kafka(
        name = "kafka",
        image = "bitnami/kafka:3.7",
        admin_image = None,
        after = [],
        env = {},
        advertised_host = None,
        port = 9092):
    host = advertised_host or name
    broker_env = merge_dicts({
        "KAFKA_CFG_NODE_ID": "1",
        "KAFKA_CFG_PROCESS_ROLES": "broker,controller",
        "KAFKA_CFG_LISTENERS": "PLAINTEXT://:9092,CONTROLLER://:9093",
        "KAFKA_CFG_ADVERTISED_LISTENERS": "PLAINTEXT://" + host + ":" + str(port),
        "KAFKA_CFG_LISTENER_SECURITY_PROTOCOL_MAP": "PLAINTEXT:PLAINTEXT,CONTROLLER:PLAINTEXT",
        "KAFKA_CFG_CONTROLLER_QUORUM_VOTERS": "1@" + host + ":9093",
        "KAFKA_CFG_CONTROLLER_LISTENER_NAMES": "CONTROLLER",
        "KAFKA_CFG_INTER_BROKER_LISTENER_NAME": "PLAINTEXT",
        "KAFKA_CFG_AUTO_CREATE_TOPICS_ENABLE": "true",
        "ALLOW_PLAINTEXT_LISTENER": "yes",
    }, env)

    broker = container.run(
        name = name,
        image = image,
        after = after,
        env = broker_env,
        ports = {"9092": port},
    )

    return {
        "container": broker,
        "name": name,
        "host": host,
        "port": port,
        "env": broker_env,
        "admin_image": admin_image or image,
        "bootstrap_servers": host + ":" + str(port),
    }
