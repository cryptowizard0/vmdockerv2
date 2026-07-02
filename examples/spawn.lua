local ao = require('ao')

Handlers.add('new', Handlers.utils.hasMatchingTag('Action', 'New'), function(msg)
    print("get New action: ", msg)

    local spawnMessage = {
        Tags = {
            { name = "Name", value = "SpawnTest" },
            { name = "Scheduler", value = "0x972AeD684D6f817e1b58AF70933dF1b4a75bfA51" },
        }
    }
    local ret = ao.spawn("4sX9Uo5-Qk37yUOMLCMrwnm4S3Wfu3Fp7QCSRN0oeoU", spawnMessage).receive()
    local process_id = ret.Tags['Process']
    print("Spawned process: ".. process_id)
end)

-- Handlers.add(
--     "Spawned",
--     Handlers.utils.hasMatchingTag("Action", "Spawned"),
--     function(msg)
--         local process_id = msg.Tags.Process
--         print("Spawned process: " .. process_id)
--     end
-- )

Handlers.add("send_eval", Handlers.utils.hasMatchingTag("Action", "SendEval"), function(msg)
    print("get SendEval")
    ao.send({
        Target = msg.SendTo,
        Action = 'Eval',
        Data = msg.Data,
    })
end)
