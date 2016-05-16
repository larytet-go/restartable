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


// typedef int (*intFunc) ();
// extern void *_cgo_init;
// int
// bridge_int_func(intFunc f)
// {
//		return f();
// }
//
// long fortytwo()
// {
//	    return (long)_cgo_init;
// }
import "C"

var addr = flag.String("l",":8881","addr")
func main() {

        log.Println("AAAA",C.fortytwo());

        log.Printf("Start Listening v2 %s",*addr)

        handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                log.Println(os.Getgid())
                fmt.Fprintf(w, "Hello, %s\n", html.EscapeString(r.URL.Path))
        })


        restartable.ListenAndServe(*addr,handler)
        
}
