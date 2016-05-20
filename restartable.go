package restartable


import (
	"net/http"
	"net"
	"log"
	"github.com/kavu/go_reuseport"
	"github.com/mailgun/manners"
	"time"
	"regexp"
	"fmt"
	"os/exec"
	"os"
	"strings"
	"debug/elf"
	"unsafe"
	"io/ioutil"
	"errors"
)

import "C"


var BuildCMD = os.Getenv("REGOBUILD") //[]string{"go","build","./main/run.go"}
var TestCMD  = os.Getenv("REGOTEST") 
var TempDIR  = os.TempDir()

//var BuildCMDRe = regexp.MustCompile("-o [^ ]")
var restarting = false
var restatrted = false

func logError(err error,v ...interface{}) {
	a := make([]interface{},0,len(v)+1)
	a = append(a,fmt.Sprintf("[ERROR] error=%s msg=",err.Error()))
	a = append(a,v...)
	log.Println(a...)
}

func logDebug(ptrn string,v ...interface{}) {
	ptrn =  "[DEBUG] msg=" + ptrn
	log.Printf(ptrn,v...)
}


func mkBuildCmd(buildCMD,tmpBin string) string {
	ss := strings.Split(buildCMD," ")
	if len(buildCMD) <= 2 {
		return buildCMD + " -o " + tmpBin
	} else {
		return strings.Join(ss[:len(ss)-1]," ") + " -o " + tmpBin + " " + ss[len(ss)-1]
	}
}

func doReload(restartCh chan int) {
	log.Println("doReload",os.Args)

	if len(TestCMD) != 0 {
		cmd := exec.Command("/bin/sh","-c",TestCMD)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Run()
		if err != nil {
			logError(err,"Test Failed")
			restartCh <- -1
			return
		}

	}
	if len(BuildCMD) != 0 {
		tmpBin, err := ioutil.TempFile(TempDIR, "rego")
		if err != nil {
			logError(err,"Can not create tmp bin")
			restartCh <- -1
			return
		}
		tmpBin.Close()
		fullCmd := mkBuildCmd(BuildCMD,tmpBin.Name())
		logDebug("Run BUILD : /bin/sh -c '%s' ",fullCmd)
		cmd := exec.Command("/bin/sh","-c",fullCmd)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			logError(err,"Build Failed")
			restartCh <- -1
			return
		}

		cmd = exec.Command("/bin/sh","-c","mv " + tmpBin.Name() + " " + os.Args[0])
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		

		if err != nil {
			logError(err,"MV Failed")
			restartCh <- -1
			return
		}

	} else {
		logDebug("NO ENV: REGOBUILD")
	}

	cmd := exec.Command(os.Args[0],os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr


	err := cmd.Run()
	if err != nil {
		logError(err,"Restart CMD")
		restartCh <- -1
	}

	log.Println("restarted")

	restartCh <- 0
}

func reloadWatcher(addr string,ch <-chan int ) {
	sleepCh := make(chan int,0)
	restartCh := make(chan int,0)
	pending := 0
	restarting := false
	//restarted  := false
	defered    := false
	for {
		select {
		case _ = <-ch:
			if ! restarting {
				pending+=1
				go func() {
					time.Sleep(1 * time.Second)
					sleepCh <- 1
				}()
			} else {
				defered = true
			}
		case ret := <-restartCh:
			restarting = false
			if ret < 0 { // Failed
				if defered {
					defered = false
					pending+=1
					sleepCh <- 1
				}
			} else {	   // OK
				//restarted = true
				return
			}
		case _ = <-sleepCh:
			pending -= 1
			if pending == 0 && ! restarting /*&& ! restarted */{
				restarting = true
				go doReload(restartCh)
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

func filesWatcher(files []string,ctrlCh chan int) {

	emask := make([]bool,len(files))
	tmask := make([]time.Time,len(files))

	processFiles := func(verbose bool) (changed bool) {
		for i, f := range files {
			s, err := os.Stat(f)
			if err != nil {
				if ! emask[i] {
					emask[i] = true
					if verbose {
						logDebug("File #d %s changed. Err(%s)", i,f,err.Error())
					}
					ctrlCh <- 1
				}
			} else {
				if emask[i] {
					emask[i] = false
					if verbose {
						logDebug("File #%d %f changed. Err(fixed)", i, f)
					}

				} else {
					if s.ModTime() != tmask[i] {
						if verbose {
							logDebug("File #%d %s changed. %v != %v", i, f, s.ModTime(), tmask[i])
						}
						tmask[i] = s.ModTime()
						changed = true
					}
				}
			}
		}
		return
	}

	processFiles(false)

	for {
		if processFiles(true) {
			ctrlCh <- 1
		}
		time.Sleep(1 * time.Second)

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


func selfReflect(filename string) ([]string,error) {
	f,err := elf.Open(filename)
	if err != nil {
		return nil,err
	}
	defer f.Close()
	syms,err := f.Symbols()
	if err != nil {
		return nil,err
	}
	var modSym elf.Symbol
	var modSymFound = false
	for _,v := range syms {
		if v.Name == "runtime.firstmoduledata" {
		//if v.Name == "github.com/kavu/go_reuseport.SimpleVar" {
		//if v.Name == "restartable.ABC" {
		//if v.Name == "encoding/base64.RawURLEncoding" {
			modSym = v
			modSymFound = true
			break
		}
	}
	if ! modSymFound {
 		return nil,errors.New("elfparse:nosym")
	}

	var datap = (*moduledata)(unsafe.Pointer(uintptr(modSym.Value)))

	files := make([]string,0)
	for i := range datap.filetab {
		bp := &datap.pclntable[datap.filetab[i]]
		file := C.GoString( (*C.char) (unsafe.Pointer(bp))  )
		if file != "<autogenerated>" && file != "@" {
			if _, err := os.Stat(file); err == nil {
				files = append(files ,file)
			}
		}
	}


	return files,nil
}

func ListenAndServe(addr string,handler http.Handler) error {
	var err error
	var affectedFiles []string

	if affectedFiles,err = selfReflect(os.Args[0]); err != nil {
		return err
	}

	if len(affectedFiles) == 0 {
		return errors.New("noaffectedFiles")
	}

	log.Printf("Start Listening v:3 on '%s'. Tracking %d files.",addr,len(affectedFiles) )

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
	go filesWatcher(affectedFiles,reloadCh )

	err = server.Serve(manners.NewListener(listener))

	logDebug("Finished")
	return err
}
