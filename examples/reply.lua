local ao = require('ao')

Handlers.add('send_message', Handlers.utils.hasMatchingTag('Action', 'SendMsg'), function(msg)
    print("get SendMsg to: " .. msg.SendTo)

    ao.send({
        Target = msg.SendTo,
        Action = 'Msg',
    }).onReply(Notify)
    
end)


Handlers.add('msg', Handlers.utils.hasMatchingTag('Action', 'Msg'), function(msg)
    print("get Msg from: " .. msg.From)

    msg.reply({
        Hello = "hello reply",
    })
end)


function Notify()
    print("receive reply")
end
