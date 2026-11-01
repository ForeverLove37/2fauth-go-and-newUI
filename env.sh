# Source this before any Go/Node command in this workspace.
export GOROOT=/home/ubuntu/rebuild/.toolchain/go
export PATH="$GOROOT/bin:/home/ubuntu/.local/node/bin:$PATH"
export GOPATH=/home/ubuntu/rebuild/.toolchain/gopath
export GOMODCACHE=/home/ubuntu/rebuild/.toolchain/gopath/pkg/mod
export GOCACHE=/home/ubuntu/rebuild/.toolchain/gocache
export GOPROXY=https://goproxy.cn,direct
export GOSUMDB=off
export GOTOOLCHAIN=local
export GOFLAGS=-mod=mod
export npm_config_cache=/home/ubuntu/rebuild/.npm-cache
