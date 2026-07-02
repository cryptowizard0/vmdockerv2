local ao = require('ao')

Handlers.add('send_message', Handlers.utils.hasMatchingTag('Action', 'SendMsg'), function(msg)
    print("get SendMsg to: " .. msg.SendTo)

    local re_msg = ao.send({
        Target = msg.SendTo,
        Action = 'Msg',
    }).receive()

    print("receive msg: " .. re_msg.Hello)
    
end)


Handlers.add('msg', Handlers.utils.hasMatchingTag('Action', 'Msg'), function(msg)
    print("get Msg from: " .. msg.From)

    --[[
    reply is same as send:
    ao.send({
        Target = msg.From,
        Hello = "hello world",
        ["X-Reference"] = msg.Reference,
    })
    ]]--

    msg.reply({
        Hello = "hello world2",
    })
end)
