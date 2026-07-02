local bint = require('.bint')(256)
local ao = require('ao')

Name = "PingPong"
Desc = "PingPong message test"
Ticker = Ticker or 'PingPong'

Handlers.add('sendping', Handlers.utils.hasMatchingTag('Action', 'SendPing'), function(msg)
    print('process ['.. ao.id .. '] get sendping from: ' .. msg.From)
    ao.send({
        Target = msg.SendTo,
        Name = Name,
        Desc = Desc,
        Id = ao.id,
        Action = 'Ping',
    })
end)

Handlers.add('ping', Handlers.utils.hasMatchingTag('Action', 'Ping'), function(msg)
    print('process ['.. ao.id .. '] get ping from: ' .. msg.From)
    ao.send({
        Target = msg.From,
        Name = Name,
        Id = ao.id,
        Action = 'Pong',
    })
end)

Handlers.add('pong', Handlers.utils.hasMatchingTag('Action', 'Pong'), function(msg)
    print('process ['.. ao.id .. '] get pong from: ' .. msg.From)
end)