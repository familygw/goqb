#!/bin/sh
# Genera los runtimes (stubs) que usa Make EXE / goqbc build para cada plataforma.
# Uso: scripts/build-stubs.sh [directorio]   (por defecto bin/stubs)
# Después: export GOQB_STUBS=$PWD/bin/stubs  (o dejarlos junto a goqb/goqbc en bin/stubs)
set -e
out=${1:-bin/stubs}
mkdir -p "$out"
for t in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
	os=${t%/*}
	arch=${t#*/}
	name="goqbrt-$os-$arch"
	[ "$os" = windows ] && name="$name.exe"
	echo "$name"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "$out/$name" ./cmd/goqbrt
done
