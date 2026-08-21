package main

import (
	"fmt"
	"os"

	"tg-log-monitor/internal/monitor"
)

func main() {
	if err := monitor.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
