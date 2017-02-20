package main

import (
    "github.com/graarh/golang-socketio"
    "github.com/graarh/golang-socketio/transport"
    "log"
)

func main() {
    log.Println("1")
    url := gosocketio.GetUrl("http://botws.generals.io", 80, false)
//    url := gosocketio.GetUrl("localhost", 80, false)
    log.Println("2")
    transport := transport.GetDefaultWebsocketTransport()
    log.Println("About to connect!")
    client, err := gosocketio.Dial(url, transport)
    if err != nil {
        log.Fatal(err)
    }
    client.On(gosocketio.OnConnection, func(c *gosocketio.Channel, args interface{}) {
        log.Println("Connected!")
    })
    
    log.Println("Closing connection")
    client.Close()

}

