package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"syscall"

	"otterindex/internal/otidxd"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:7337", "listen address (tcp)")
	flag.Parse()

	s := otidxd.NewServer(otidxd.Options{Listen: *listen})
	if err := s.Run(); err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			_, _ = fmt.Fprintf(os.Stderr, "listen address in use: %s\nTry: -listen 127.0.0.1:7338\n", *listen)
		} else {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
