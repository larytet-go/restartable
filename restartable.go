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
	"debug/elf"
	"unsafe"
	"errors"
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

type method struct {
	name    *string
	pkgpath *string
	mtyp    *_type
	typ     *_type
	ifn     unsafe.Pointer
	tfn     unsafe.Pointer
}

type uncommontype struct {
	name    *string
	pkgpath *string
	mhdr    []method
}

type _type struct {
	size       uintptr
	ptrdata    uintptr // size of memory prefix holding all pointers
	hash       uint32
	_unused    uint8
	align      uint8
	fieldalign uint8
	kind       uint8
	alg        *typeAlg
					   // gcdata stores the GC type data for the garbage collector.
					   // If the KindGCProg bit is set in kind, gcdata is a GC program.
					   // Otherwise it is a ptrmask bitmap. See mbitmap.go for details.
	gcdata  *byte
	_string *string
	x       *uncommontype
	ptrto   *_type
}

type functab struct {
	entry   uintptr
	funcoff uintptr
}

type modulehash struct {
	modulename   string
	linktimehash string
	runtimehash  *string
}
type bitvector struct {
	n        int32 // # of bits
	bytedata *uint8
}

// typeAlg is also copied/used in reflect/type.go.
// keep them in sync.
type typeAlg struct {
	// function for hashing objects of this type
	// (ptr to object, seed) -> hash
	hash func(unsafe.Pointer, uintptr) uintptr
	// function for comparing objects of this type
	// (ptr to object A, ptr to object B) -> ==?
	equal func(unsafe.Pointer, unsafe.Pointer) bool
}


type moduledata struct {
	pclntable    []byte
	ftab         []functab
	filetab      []uint32
	findfunctab  uintptr
	minpc, maxpc uintptr

	text, etext           uintptr
	noptrdata, enoptrdata uintptr
	data, edata           uintptr
	bss, ebss             uintptr
	noptrbss, enoptrbss   uintptr
	end, gcdata, gcbss    uintptr

	typelinks []*_type

	modulename   string
	modulehashes []modulehash

	gcdatamask, gcbssmask bitvector

	next *moduledata
}

func selfReflect(filename string) error {
	f,err := elf.Open(filename)
	if err != nil {
		return err
	}
	syms,err := f.Symbols()
	if err != nil {
		return err
	}
	var modSym elf.Symbol
	var modSymFound = false
	for _,v := range syms {
		if v.Name == "runtime.firstmoduledata" {
		//if v.Name == "restartable.ABC" {
		//if v.Name == "encoding/base64.RawURLEncoding" {
			modSym = v
			modSymFound = true
			break
		}
	}
	if ! modSymFound
 		return errors.New("elfparse:nosym")
	}

	log.Printf("%v", f.Sections[18] )
	log.Printf("%08x %08x", f.Sections[18].Addr,f.Sections[18].Offset )
	//log.Printf("KINDA %s\n",modSym.Section)
	log.Printf("%08x\n",modSym.Value)
	//var realP = uint64(uintptr(unsafe.Pointer(&base64.RawURLEncoding)))
	var backp = (*moduledata)(unsafe.Pointer(uintptr(modSym.Value)))

	log.Printf("MODULE '%s'",(*backp).modulename)

	log.Printf("MODULE '%v'",(*backp).next)

	return nil
}

func ListenAndServe(addr string,handler http.Handler,) error {
	if err := selfReflect(os.Args[0]); err != nil {
		return err
	}

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
