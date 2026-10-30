//go:build windows || linux || darwin

package main

import (
	"errors"
	"flag"
	"os"

	"github.com/armanlife/firefox-vpn-client/core/internal/tunservice"
)

// run runs a command. The init system (the service manager, systemd,
// launchd) starts "service"; the elevated copy of an install or uninstall
// runs "install-as-admin" or "uninstall-as-admin".
func run(command string, args []string) int {
	switch command {
	case "install", "uninstall":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		prompt := fs.String("prompt", "", "the reason the administrator prompt shows (macOS)")
		if fs.Parse(args) != nil {
			return exitUsage
		}
		var err error
		if command == "install" {
			err = tunservice.InstallForUser(*prompt)
		} else {
			err = tunservice.UninstallForUser(*prompt)
		}
		if errors.Is(err, tunservice.ErrCancelled) {
			return exitCancelled
		}
		if err != nil {
			return fail(err)
		}
	case "update":
		self, err := os.Executable()
		if err == nil {
			err = tunservice.UpdateService(self)
		}
		if err != nil {
			return fail(err)
		}
	case "service":
		if err := tunservice.RunService(); err != nil {
			return fail(err)
		}
	case "install-as-admin", "uninstall-as-admin":
		fs := flag.NewFlagSet(command, flag.ContinueOnError)
		user := fs.String("user", "", "the user who may use the service: a SID on Windows, a user id elsewhere")
		report := fs.String("report", "", "where to write why it failed")
		if fs.Parse(args) != nil {
			return exitUsage
		}
		var err error
		if command == "install-as-admin" {
			if *user == "" {
				return exitUsage
			}
			err = tunservice.Install(*user)
		} else {
			err = tunservice.Uninstall()
		}
		if err != nil {
			tunservice.WriteReport(*report, err)
			return fail(err)
		}
	default:
		return runPlatform(command, args)
	}
	return 0
}
