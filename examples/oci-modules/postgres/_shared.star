def merge_dicts(base, overrides):
    merged = {}
    for key, value in base.items():
        merged[key] = value
    for key, value in overrides.items():
        merged[key] = value
    return merged

def append_unique(values, item):
    output = []
    seen = False
    for value in values:
        output.append(value)
        if value == item:
            seen = True
    if not seen:
        output.append(item)
    return output

def merge_after(database, after):
    merged = []
    for item in after:
        merged = append_unique(merged, item)
    return append_unique(merged, database["name"])

def sanitize_name(value):
    output = ""
    for ch in str(value):
        if ("a" <= ch and ch <= "z") or ("A" <= ch and ch <= "Z") or ("0" <= ch and ch <= "9"):
            output += ch.lower()
        else:
            output += "-"
    while "--" in output:
        output = output.replace("--", "-")
    if output.startswith("-"):
        output = output[1:]
    if output.endswith("-"):
        output = output[:-1]
    return output or "query"

def quoted(value):
    return "'" + str(value).replace("'", "'\"'\"'") + "'"

def sql_value(value):
    if value == None:
        return "null"
    if type(value) == "bool":
        if value:
            return "true"
        return "false"
    if type(value) == "int":
        return str(value)
    if type(value) == "float":
        return str(value)
    return "'" + str(value).replace("'", "''") + "'"

def sql_predicate(where):
    if type(where) == "string":
        return where
    parts = []
    for key, value in where.items():
        parts.append(str(key) + " = " + sql_value(value))
    return " and ".join(parts)
