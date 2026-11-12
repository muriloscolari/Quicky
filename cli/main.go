package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime)
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "quicky:", err)
		os.Exit(1)
	}
}
