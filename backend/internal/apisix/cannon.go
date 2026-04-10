package apisix

// TrafficCannonPluginName is the APISIX plugin name registered in apisix.yaml.
const TrafficCannonPluginName = "babelsuite-traffic-cannon"

// TrafficCannonTriggerRoute is the URI that the BabelSuite runner POSTs to.
const TrafficCannonTriggerRoute = "/_babelsuite/traffic/start"

// TrafficCannonLua is the OpenResty/Lua plugin embedded verbatim into the
// generated apisix.yaml.  It drives outbound load generation entirely inside
// the APISIX sidecar process — no extra processes or sidecar containers needed.
//
// Execution models:
//   - Closed (virtual users): ngx.thread.spawn() per user, looping until stage duration expires.
//   - Open   (constant RPS):  ngx.timer.every() at 1/rps interval.
//
// Latency samples are accumulated in ngx.shared.dict "babelsuite_traffic" as
// 4-byte big-endian fixed-point (ms × 10) so all Nginx worker processes share
// a single lock-safe histogram without needing a Redis sidecar.
//
// The plugin responds to POST /_babelsuite/traffic/start with a blocking
// synchronous JSON result once the full run completes:
//
//	{"total":N,"failures":N,"p50":X,"p95":X,"p99":X,"min":X,"max":X}
const TrafficCannonLua = `
local json     = require("cjson")
local http_lib = require("resty.http")
local shared   = ngx.shared.babelsuite_traffic

-- ── helpers ────────────────────────────────────────────────────────────────────

local function record(sampler, latency_ms, ok)
    local key_count = sampler .. ":count"
    local key_fail  = sampler .. ":fail"
    local key_lats  = sampler .. ":lats"
    shared:incr(key_count, 1, 0)
    if not ok then shared:incr(key_fail, 1, 0) end
    -- pack latency as 4-byte big-endian uint32 (ms * 10)
    local v  = math.min(math.floor(latency_ms * 10), 4294967295)
    local b3 = math.floor(v / 16777216) % 256
    local b2 = math.floor(v /    65536) % 256
    local b1 = math.floor(v /      256) % 256
    local b0 = v % 256
    local existing = shared:get(key_lats) or ""
    shared:set(key_lats, existing .. string.char(b3, b2, b1, b0))
end

local function decode_lats(raw)
    local out = {}
    for i = 1, #raw, 4 do
        local b3, b2, b1, b0 = raw:byte(i, i + 3)
        out[#out + 1] = (b3 * 16777216 + b2 * 65536 + b1 * 256 + b0) / 10.0
    end
    table.sort(out)
    return out
end

local function percentile(sorted, p)
    if #sorted == 0 then return 0 end
    return sorted[math.max(1, math.ceil(#sorted * p / 100))]
end

local function collect_results(sampler_names)
    local all_lats = {}
    local total_count, total_fail = 0, 0
    local samplers = {}
    for _, name in ipairs(sampler_names) do
        local count = tonumber(shared:get(name .. ":count")) or 0
        local fail  = tonumber(shared:get(name .. ":fail"))  or 0
        local lats  = decode_lats(shared:get(name .. ":lats") or "")
        total_count = total_count + count
        total_fail  = total_fail  + fail
        for _, v in ipairs(lats) do all_lats[#all_lats + 1] = v end
        samplers[name] = {
            count    = count,
            failures = fail,
            p50      = percentile(lats, 50),
            p95      = percentile(lats, 95),
            p99      = percentile(lats, 99),
            min      = lats[1]      or 0,
            max      = lats[#lats]  or 0,
        }
    end
    table.sort(all_lats)
    return {
        total    = total_count,
        failures = total_fail,
        p50      = percentile(all_lats, 50),
        p95      = percentile(all_lats, 95),
        p99      = percentile(all_lats, 99),
        min      = all_lats[1]         or 0,
        max      = all_lats[#all_lats] or 0,
        samplers = samplers,
    }
end

-- ── closed model  (virtual users) ─────────────────────────────────────────────

local function run_user_loop(target, tasks, duration_s, sampler)
    local deadline = ngx.now() + duration_s
    while ngx.now() < deadline do
        local task  = tasks[math.random(#tasks)]
        local httpc = http_lib.new()
        httpc:set_timeout(15000)
        local t0 = ngx.now()
        local res, err = httpc:request_uri(target .. (task.path or "/"), {
            method  = task.method  or "GET",
            body    = task.body    or nil,
            headers = task.headers or {},
        })
        local lat = (ngx.now() - t0) * 1000
        record(sampler, lat, err == nil and res ~= nil and res.status < 500)
        ngx.sleep(0)
    end
end

local function run_closed_model(cfg, sampler_names)
    local target = cfg.target
    local tasks  = {}
    for _, u in ipairs(cfg.users_def or {}) do
        for _, t in ipairs(u.tasks or {}) do tasks[#tasks + 1] = t end
    end
    if #tasks == 0 then tasks = {{method = "GET", path = "/"}} end

    for si, stage in ipairs(cfg.stages or {}) do
        local n_users = stage.users    or 1
        local dur_s   = stage.duration or 10
        local sampler = sampler_names[si] or ("stage_" .. si)
        local threads = {}
        for i = 1, n_users do
            threads[i] = ngx.thread.spawn(run_user_loop, target, tasks, dur_s, sampler)
        end
        for _, th in ipairs(threads) do ngx.thread.wait(th) end
        if stage.stop then break end
    end
end

-- ── open model  (constant RPS) ────────────────────────────────────────────────

local _open_active = false
local _open_tasks  = {}
local _open_target = ""
local _open_sample = "open_model"

local function open_tick(premature)
    if premature or not _open_active then return end
    local task  = _open_tasks[math.random(#_open_tasks)]
    local httpc = http_lib.new()
    httpc:set_timeout(10000)
    local t0 = ngx.now()
    local res, err = httpc:request_uri(_open_target .. (task.path or "/"), {
        method  = task.method  or "GET",
        body    = task.body    or nil,
        headers = task.headers or {},
    })
    local lat = (ngx.now() - t0) * 1000
    record(_open_sample, lat, err == nil and res ~= nil and res.status < 500)
end

local function run_open_model(cfg)
    local rps     = math.max(cfg.rps or 1, 0.001)
    local total_s = 0
    for _, stage in ipairs(cfg.stages or {}) do
        total_s = total_s + (stage.duration or 10)
    end
    if total_s == 0 then total_s = 60 end

    _open_active = true
    _open_target = cfg.target
    _open_tasks  = {}
    for _, u in ipairs(cfg.users_def or {}) do
        for _, t in ipairs(u.tasks or {}) do _open_tasks[#_open_tasks + 1] = t end
    end
    if #_open_tasks == 0 then _open_tasks = {{method = "GET", path = "/"}} end

    ngx.timer.every(1.0 / rps, open_tick)
    ngx.sleep(total_s)
    _open_active = false
end

-- ── plugin entry point ─────────────────────────────────────────────────────────

local _M = {}
_M.version  = 0.1
_M.priority = 10
_M.name     = "babelsuite-traffic-cannon"
_M.schema   = {type = "object", properties = {}, additionalProperties = false}

function _M.access(conf, ctx)
    if ngx.var.uri ~= "/_babelsuite/traffic/start" then
        return ngx.exit(404)
    end
    if ngx.req.get_method() ~= "POST" then
        return ngx.exit(405)
    end

    ngx.req.read_body()
    local ok_p, cfg = pcall(json.decode, ngx.req.get_body_data() or "{}")
    if not ok_p or type(cfg) ~= "table" then
        ngx.status = 400
        ngx.say(json.encode({error = "invalid config JSON"}))
        return ngx.exit(400)
    end

    shared:flush_all()

    local model         = cfg.model or "closed"
    local sampler_names = {}
    for si = 1, #(cfg.stages or {{}}) do
        sampler_names[si] = (model == "open") and "open_model" or ("stage_" .. si)
    end

    if model == "open" then
        run_open_model(cfg)
    else
        run_closed_model(cfg, sampler_names)
    end

    local results = collect_results(sampler_names)
    ngx.status = 200
    ngx.header["Content-Type"] = "application/json"
    ngx.say(json.encode(results))
    return ngx.exit(200)
end

return _M
`
