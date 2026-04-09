load("@babelsuite/runtime", "task")
load("_shared.star", "merge_after", "merge_dicts", "quoted", "sanitize_name", "sql_predicate", "sql_value")

def query_task(database, name, sql, after = [], env = {}):
    task_env = merge_dicts(database["env"], env)
    return task.run(
        name = name,
        image = database["client_image"],
        after = merge_after(database, after),
        env = task_env,
        command = [
            "bash",
            "-lc",
            "psql "
            + quoted(database["url"])
            + " -v ON_ERROR_STOP=1 -c "
            + quoted(sql),
        ],
    )

def connect(database, after = []):
    return query_task(
        database,
        name = database["name"] + "-connect",
        sql = "select 1;",
        after = after,
    )

def query(database, sql, name = None, after = []):
    return query_task(
        database,
        name = name or (database["name"] + "-query-" + sanitize_name(sql[:24])),
        sql = sql,
        after = after,
    )

def insert(database, table, values, after = []):
    columns = []
    literal_values = []
    for key, value in values.items():
        columns.append(str(key))
        literal_values.append(sql_value(value))
    return query_task(
        database,
        name = database["name"] + "-insert-" + sanitize_name(table),
        sql = "insert into " + table + " (" + ", ".join(columns) + ") values (" + ", ".join(literal_values) + ");",
        after = after,
    )

def select(database, table, columns = ["*"], where = None, after = []):
    sql = "select " + ", ".join(columns) + " from " + table
    if where != None:
        sql += " where " + sql_predicate(where)
    sql += ";"
    return query_task(
        database,
        name = database["name"] + "-select-" + sanitize_name(table),
        sql = sql,
        after = after,
    )

def delete(database, table, where, after = []):
    return query_task(
        database,
        name = database["name"] + "-delete-" + sanitize_name(table),
        sql = "delete from " + table + " where " + sql_predicate(where) + ";",
        after = after,
    )

def upsert(database, table, values, conflict_columns, after = []):
    columns = []
    literal_values = []
    assignments = []
    for key, value in values.items():
        columns.append(str(key))
        literal_values.append(sql_value(value))
        assignments.append(str(key) + " = excluded." + str(key))
    return query_task(
        database,
        name = database["name"] + "-upsert-" + sanitize_name(table),
        sql = (
            "insert into " + table
            + " (" + ", ".join(columns) + ")"
            + " values (" + ", ".join(literal_values) + ")"
            + " on conflict (" + ", ".join(conflict_columns) + ")"
            + " do update set " + ", ".join(assignments)
            + ";"
        ),
        after = after,
    )
