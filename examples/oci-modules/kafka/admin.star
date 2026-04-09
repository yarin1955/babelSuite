load("@babelsuite/runtime", "task")
load("_shared.star", "merge_dicts", "merge_after", "quoted", "sanitize_name")

def admin_task(cluster, name, script_body, after = [], env = {}):
    task_env = merge_dicts(cluster["env"], env)
    return task.run(
        name = name,
        image = cluster["admin_image"],
        after = merge_after(cluster, after),
        env = task_env,
        command = ["bash", "-lc", script_body],
    )

def create_topic(cluster, topic, partitions = 1, replication_factor = 1, configs = {}, after = []):
    config_flags = ""
    for key, value in configs.items():
        config_flags += " --config " + quoted(str(key) + "=" + str(value))
    return admin_task(
        cluster,
        name = cluster["name"] + "-create-topic-" + sanitize_name(topic),
        script_body = (
            "kafka-topics.sh"
            + " --bootstrap-server " + quoted(cluster["bootstrap_servers"])
            + " --create"
            + " --if-not-exists"
            + " --topic " + quoted(topic)
            + " --partitions " + str(partitions)
            + " --replication-factor " + str(replication_factor)
            + config_flags
        ),
        after = after,
    )

def delete_topic(cluster, topic, after = []):
    return admin_task(
        cluster,
        name = cluster["name"] + "-delete-topic-" + sanitize_name(topic),
        script_body = (
            "kafka-topics.sh"
            + " --bootstrap-server " + quoted(cluster["bootstrap_servers"])
            + " --delete"
            + " --if-exists"
            + " --topic " + quoted(topic)
        ),
        after = after,
    )

def set_group_offset(cluster, group, topic, offset, partition = 0, after = []):
    return admin_task(
        cluster,
        name = cluster["name"] + "-offset-" + sanitize_name(group) + "-" + sanitize_name(topic),
        script_body = (
            "kafka-consumer-groups.sh"
            + " --bootstrap-server " + quoted(cluster["bootstrap_servers"])
            + " --group " + quoted(group)
            + " --topic " + quoted(topic + ":" + str(partition))
            + " --reset-offsets"
            + " --to-offset " + str(offset)
            + " --execute"
        ),
        after = after,
    )

def disconnect(cluster):
    return admin_task(
        cluster,
        name = cluster["name"] + "-disconnect",
        script_body = "kafka-server-stop.sh || true",
    )
