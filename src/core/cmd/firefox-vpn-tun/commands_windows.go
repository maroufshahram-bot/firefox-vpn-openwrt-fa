package main

import (
	"errors"
	"strconv"

	"github.com/armanlife/firefox-vpn-client/core/internal/tunservice"
)

// runPlatform runs the commands an update and an uninstall from inside the
// service leave behind: "restart-service" and "cleanup".
func runPlatform(command string, args []string) int {
	switch command {
	case "restart-service":
		pid, err := pidArg(args)
		if err != nil {
			return exitUsage
		}
		if err := tunservice.RestartService(pid); err != nil {
			return fail(err)
		}
	case "cleanup":
		pid, err := pidArg(args)
		if err != nil || len(args) != 2 {
			return exitUsage
		}
		if err := tunservice.Cleanup(pid, args[1]); err != nil {
			return fail(err)
		}
	default:
		usage()
	}
	return 0
}

func pidArg(args []string) (int, error) {
	if len(args) == 0 {
		return 0, errors.New("no process id")
	}
	return strconv.Atoi(args[0])
}
