load("@babelsuite/runtime", "service")
load("_shared.star", "merge_dicts")

def pg(
        name = "db",
        image = "postgres:16",
        client_image = None,
        after = [],
        env = {},
        database = "app",
        username = "postgres",
        password = "postgres",
        port = 5432):
    database_env = merge_dicts({
        "POSTGRES_DB": database,
        "POSTGRES_USER": username,
        "POSTGRES_PASSWORD": password,
    }, env)

    db = service.run(
        name = name,
        image = image,
        after = after,
        env = database_env,
        ports = {"5432": port},
    )

    return {
        "service": db,
        "name": name,
        "host": name,
        "port": port,
        "env": database_env,
        "client_image": client_image or image,
        "database": database,
        "username": username,
        "password": password,
        "url": "postgres://" + username + ":" + password + "@" + name + ":" + str(port) + "/" + database,
    }
