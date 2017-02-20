package main

import (
    "github.com/graarh/golang-socketio"
    "github.com/graarh/golang-socketio/transport"
    "log"
    //"flag"
  //  "net/url"
)

func main() {
    // TODO: what is `flag` anyway?
    //serverAddr := flag.String("server-url", "ws://botws.generals.io", "address of generals-api server")
//    controllerUrl, err := url.Parse(*serverAddr)
    log.Println("1")
//    url := gosocketio.GetUrl("http://botws.generals.io", 80, false)
//    url := gosocketio.GetUrl("localhost", 80, false)
    log.Println("2")
    transport := transport.GetDefaultWebsocketTransport()
    log.Println("About to connect!")
    //client, err := gosocketio.Dial(url, transport)
    client, err := gosocketio.Dial("ws://botws.generals.io", transport)
    log.Println("Oh hello")
    if err != nil {
        log.Println("Hmmm")
        log.Fatal(err)
    }
    client.On(gosocketio.OnConnection, func(c *gosocketio.Channel, args interface{}) {
        log.Println("Connected!")
    })
    
    log.Println("Closing connection")
    client.Close()

}

