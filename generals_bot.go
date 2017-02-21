package main

import (
    "github.com/graarh/golang-socketio"
    "github.com/graarh/golang-socketio/transport"
    "log"
)

func main() {
    transport := transport.GetDefaultWebsocketTransport()
    ws_url := "ws://botws.generals.io/socket.io/?EIO=3&transport=websocket"
    client, err := gosocketio.Dial(ws_url, transport)
    defer client.Close()
    if err != nil {
        log.Fatal(err)
    }
    client.On(gosocketio.OnConnection, func(ws_channel *gosocketio.Channel, args interface{}) {
        log.Println("Connected!")
       // user_id := "upcodes_abc123"
       // username := "UpCodes Bot"
        type MyEventData struct {
            user_id string
            username string
        }
    
        //ws_channel.Emit("set_username", MyEventData{user_id, username})
//        err := ws_channel.Emit("set_username", MyEventData{user_id, username})
        err := ws_channel.Emit("", "['set_username', 'upcodes_abc123', 'UpCodes Bot']")
        if err != nil {
            log.Println(err)
        }
        log.Println("Set username")

        custom_game_id := "upcodes_game"
        type MyEventData2 struct {
            custom_game_id string
            user_id string
        }
//        ws_channel.Emit("join_private", MyEventData2{custom_game_id, user_id})
        ws_channel.Emit("", "['join_private', 'upcodes_game', 'upcodes_abc123']")
//        ws_channel.Emit("", []string{"join_private", custom_game_id, user_id})
        log.Printf("Joined custom game at http://bot.generals.io/games/%v", custom_game_id)
        //socket.emit('set_force_start', custom_game_id, true);

    })

    

    
    select {}
    
    
}

