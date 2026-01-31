package main

import (
	"flag"
	"fmt"
	"os"

	"otterindex/internal/otidxd"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:7337", "listen address (tcp)")
	flag.Parse()

	s := otidxd.NewServer(otidxd.Options{Listen: *listen})
	if err := s.Run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
