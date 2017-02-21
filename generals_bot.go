package main

import (
    "github.com/graarh/golang-socketio"
    "github.com/graarh/golang-socketio/transport"
    "log"
    "time"
)

func main() {
    transport := transport.GetDefaultWebsocketTransport()
    client, err := gosocketio.Dial("ws://botws.generals.io/socket.io/?EIO=3&transport=websocket", transport)
    if err != nil {
        log.Fatal(err)
    }
    client.On(gosocketio.OnConnection, func(c *gosocketio.Channel, args interface{}) {
        log.Println("Connected!")
    })

    duration := time.Duration(10)*time.Second
    time.Sleep(duration)
    
    client.Close()
}

