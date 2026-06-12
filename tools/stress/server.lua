-- Server state — created once, lives for entire test
local metrics = {total_requests = 0, errors = 0}
local cache = {}  -- request cache

function handle_request(req_id, data)
    metrics.total_requests = metrics.total_requests + 1
    
    -- Simulate processing (string ops, table ops, logic)
    local result = {}
    for i = 1, 20 do
        result[i] = {
            id = req_id .. "_" .. i,
            value = math.random(1000),
            name = string.format("item_%d_%d", req_id, i),
            tags = {"processed", "active", "cached"}
        }
    end
    
    -- Pattern matching
    local status = string.match(data or "", "status=(%d+)")
    if status and tonumber(status) >= 500 then
        metrics.errors = metrics.errors + 1
    end
    
    return result
end

function get_metrics()
    return metrics
end
