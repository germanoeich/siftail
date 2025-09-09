package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `siftail - a TUI for tailing and exploring logs

USAGE:
  siftail [flags] [file]       # file mode - tail a file
  siftail docker               # docker mode - stream from all running containers
  <command> | siftail          # stdin mode - read piped input as live stream

EXAMPLES:
  siftail /var/log/app.log     # tail a file with rotation awareness
  siftail docker               # stream from all Docker containers
  journalctl -f | siftail      # tail systemd journal via stdin

FLAGS:
  -h, --help    show this help message

HOTKEYS (once running):
  q             quit
  h             highlight text (no scroll)
  f             find text (jump to matches)
  F             filter-in (show only matching lines)
  U             filter-out (hide matching lines)
  1-9           toggle severity levels
  l             list containers (docker mode)
  P             manage presets (docker mode)
`

func main() {
	var help bool
	flag.BoolVar(&help, "h", false, "show help")
	flag.BoolVar(&help, "help", false, "show help")
	flag.Usage = func() {
		fmt.Print(usage)
	}
	flag.Parse()

	if help {
		flag.Usage()
		return
	}

	args := flag.Args()

	// Determine mode based on arguments
	if len(args) == 0 {
		// Check if stdin has data (piped input)
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			fmt.Println("siftail: stdin mode - reading piped input (not yet implemented)")
		} else {
			fmt.Println("siftail: no input specified. Use -h for help.")
			os.Exit(1)
		}
	} else if len(args) == 1 && args[0] == "docker" {
		fmt.Println("siftail: docker mode - streaming from containers (not yet implemented)")
	} else if len(args) == 1 {
		fmt.Printf("siftail: file mode - tailing %s (not yet implemented)\n", args[0])
	} else {
		fmt.Println("siftail: invalid arguments. Use -h for help.")
		os.Exit(1)
	}
}
