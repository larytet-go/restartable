package restartable


import (
	"net/http"
	"net"
	"log"
	"github.com/kavu/go_reuseport"
	"github.com/mailgun/manners"
	"golang.org/x/exp/inotify"
	"time"
	"regexp"
	"fmt"
	"os/exec"
	"runtime"
	"os"
)

var BuildCMD = []string{"go","test","./..."}
var TestCMD  = []string{"go","test","./..."}

func logError(err error,v ...interface{}) {
	a := make([]interface{},0,len(v)+1)
	a = append(a,err)
	a = append(a,v...)
	log.Println(fmt.Sprintf("[ERROR] error=%s msg=" ,a...))
}

func doReload() {
	log.Println("doReload",os.Args)

	if len(BuildCMD) != 0 {
		cmd := exec.Command(os.Args[0],os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Run()
		if err != nil {
			logError(err,"Build Failed")
			return
		}

	}

	cmd := exec.Command(os.Args[0],os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		logError(err,"Restart CMD")
	}

	log.Println("restarted",err)
	//fmt.Fprintf(conn, "GET /close HTTP/1.0\r\n\r\n")
	//conn.Close()
}

func reloadWatcher(addr string,ch <-chan int ) chan int {
	sleepCh := make(chan int,0)
	pending := 0

	for {
		select {
		case _ = <-ch:
			pending+=1
			go func() {
				time.Sleep(1 * time.Second)
				sleepCh <- 1
			}()
		case _ = <-sleepCh:
			pending -= 1
			if pending == 0 {
				doReload()
			}
		}
	}

}

var testPatterns = []*regexp.Regexp {
	regexp.MustCompile(`.*\.go`),
}

func passTest(name string) bool {
	for _,p := range testPatterns {
		if p.MatchString(name) {
			return true
		}
	}
	return false
}

func filesWatcher(dir string,ctrlCh chan int) {
	watcher, err := inotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	err = watcher.Watch(dir)
	if err != nil {
		log.Fatal(err)
	}
	for {
		select {
		case ev := <-watcher.Event:
			log.Println(ev,ev.Name,passTest(ev.Name))
			if ( ev.Mask & (inotify.IN_OPEN | inotify.IN_CLOSE_NOWRITE) == 0 ) {
				if passTest(ev.Name) {
					ctrlCh <- 1
				}
			}
		case err := <-watcher.Error:
			log.Printf("error:%s", err)
		}
	}
}

func ListenAndServe(addr string,handler http.Handler,) error {

	resetOldConn,  _ := net.Dial("tcp", addr)

	listener, err := reuseport.NewReusablePortListener("tcp4", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	if resetOldConn != nil {
		fmt.Fprintf(resetOldConn, "GET /close HTTP/1.0\r\n\r\n")
		resetOldConn.Close()
	}

	server := manners.NewServer()

	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch (r.URL.Path ) {
		case "/close" :
			server.Close()
		default:
			handler.ServeHTTP(w,r)
		}
	})

	reloadCh  := make(chan int,0)

	go reloadWatcher(addr,reloadCh )
	go filesWatcher("./",reloadCh )

	runtime.Caller(1)

	return server.Serve(manners.NewListener(listener))
}
