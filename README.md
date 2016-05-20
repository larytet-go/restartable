# restartable

export REGOBUILD="cd /mnt/GOROOT/src/github.com/martende/restartable && go build main/run.go"
export REGOTEST =""

go build main/run.go
./run
