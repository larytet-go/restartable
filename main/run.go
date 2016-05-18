package main

import (
  "log"
  "html"
  "net/http"
  "os"
  "flag"
  "fmt"
        "github.com/martende/restartable"
    "time"
)


var addr = flag.String("l",":8881","addr")
func main() {
        time.Sleep(1000 * time.Second)

        handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                log.Println(os.Getgid())
                fmt.Fprintf(w, "Hello, v2 %s\n", html.EscapeString(r.URL.Path))
        })


        err := restartable.ListenAndServe(*addr,handler)
        if err != nil {
                panic(err)
        }
}
