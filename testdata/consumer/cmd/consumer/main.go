package main

import (
	"fmt"
	"os"
	"path/filepath"

	gorattler "github.com/dbast/go-rattler"
)

func main() {
	comparison, err := gorattler.CompareVersions("1.0rc1", "1.0")
	if err != nil || comparison != -1 {
		fmt.Fprintf(os.Stderr, "compare versions: %d, %v\n", comparison, err)
		os.Exit(1)
	}
	counts, err := gorattler.LockEnvironment(filepath.Join("testdata", "pixi.lock"), "default", "osx-arm64")
	if err != nil || counts != (gorattler.PackageCounts{Conda: 1, PyPI: 1}) {
		fmt.Fprintf(os.Stderr, "lock environment: %+v, %v\n", counts, err)
		os.Exit(1)
	}
}
