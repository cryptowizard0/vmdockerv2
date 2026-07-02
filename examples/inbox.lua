local ao = require('ao')

Handlers.add('send_message', Handlers.utils.hasMatchingTag('Action', 'SendMsg'), function(msg)
    print("get SendMsg to: " .. msg.SendTo)

    local re_msg = ao.send({
        Target = msg.SendTo,
        Action = 'Msg',
    })
    
end)

Handlers.add('msg', Handlers.utils.hasMatchingTag('Action', 'Msg'), function(msg)
    print("get Msg from: " .. msg.From)

    ao.send({
        Target = msg.From,
        Action = 'ReciveMsg',
        Data = "receive 1",
    })

    ao.send({
        Target = msg.From,
        Action = 'ReciveMsg',
        Data = "receive 2",
    })


end)