package main

import (
	"fmt"
	"go/importer"
	"testing"
)

func TestSTH(t *testing.T){
	pkg, err := importer.Default().Import("time")
	if err != nil {
		fmt.Println(err)
	}
	for _, declName := range pkg.Scope().Names() {
		fmt.Println(declName)
	}
}
