// This fixture is used only by the Pulumi preview tests. Every unsupported
// command fails, so a preview can never create, resize, or delete a real VM.
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	command := strings.Join(os.Args[1:], " ")
	log, err := os.OpenFile(os.Getenv("PULUMI_TEST_COMMAND_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	if _, err := fmt.Fprintln(log, command); err != nil {
		panic(err)
	}
	if err := log.Close(); err != nil {
		panic(err)
	}
	switch command {
	case "version --format json":
		fmt.Println(`{"multipass":"1.16.0","multipassd":"1.16.0"}`)
	case "find --format json":
		fmt.Println(`{"images":{"24.04":{"aliases":["lts","noble"],"os":"Ubuntu","release":"24.04 LTS","remote":"release","version":"20260401"}}}`)
	default:
		fmt.Fprintf(os.Stderr, "unexpected CLI command during preview: %s\n", command)
		os.Exit(1)
	}
}
