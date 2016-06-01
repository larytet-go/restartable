# restartable

Silently restartable server that listens to changes on it's own sources.

1 ) After start . Reads itself and finds out all sources that were used for compilation

2 ) If something has changed starts build cmd from $REGOBUILD environment. And test command from $REGOTEST environment varibale.

3 ) If tests successfully passed. Starts a new copy of itself and does not serve a new connection , after serving the last active connection exists.

Usage: 
```
export REGOBUILD="cd /mnt/GOROOT/src/github.com/martende/restartable && go build main/run.go"
  
export REGOTEST =""

go build main/run.go
./run
```
