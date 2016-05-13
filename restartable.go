package main

import (
  "log"
  "html"
  "net/http"
  "os"
  "github.com/kavu/go_reuseport"
  "github.com/mailgun/manners"
  "flag"
  "fmt"
)

var addr = flag.String("l",":8881","addr")
func main() {
        log.Printf("Start Listening v2 %s",*addr)

        listener, err := reuseport.NewReusablePortListener("tcp4", *addr)
        if err != nil {
                panic(err)
        }
        defer listener.Close()
        
        server := manners.NewServer()

        handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                switch (r.URL.Path ) {
                case "/close" :
                        log.Println("GONADIE")
                        server.Close()
                default:
                        log.Println(os.Getgid())
                        fmt.Fprintf(w, "Hello, %s\n", html.EscapeString(r.URL.Path))
                }
        })

        server.Handler = handler

        //oldListener, _ := net.Listen("tcp", ":8882")

        //server.ListenAndServe()
        server.Serve(manners.NewListener(listener))

}
