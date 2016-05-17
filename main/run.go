package main

import (
  "log"
  "html"
  "net/http"
  "os"
  "flag"
  "fmt"
        "github.com/martende/restartable"
)


var addr = flag.String("l",":8881","addr")
func main() {

        log.Printf("Start Listening v2 %s",*addr)

        handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                log.Println(os.Getgid())
                fmt.Fprintf(w, "Hello, %s\n", html.EscapeString(r.URL.Path))
        })


        restartable.ListenAndServe(*addr,handler)
}
