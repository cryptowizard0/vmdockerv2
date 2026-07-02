local ao = require('ao')

Handlers.add('send_message', Handlers.utils.hasMatchingTag('Action', 'SendMsg'), function(msg)
    print("get SendMsg to: " .. msg.SendTo)

    ao.send({
        Target = msg.SendTo,
        Action = 'Msg',
    })

    local re_msg = Receive({From = msg.SendTo})
    print("receive msg: " .. re_msg.Hello)
    
end)

Handlers.add('msg', Handlers.utils.hasMatchingTag('Action', 'Msg'), function(msg)
    print("get Msg from: " .. msg.From)

    ao.send({
        Target = msg.From,
        Hello = "hello world",
    })
end)