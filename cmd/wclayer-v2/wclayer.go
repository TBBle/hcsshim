package main

import (
	"fmt"
	"os"

	"github.com/Microsoft/hcsshim/osversion"
	"github.com/urfave/cli"
)

// Add a manifest to get proper Windows version detection.
//
// goversioninfo can be installed with "go get github.com/josephspurrier/goversioninfo/cmd/goversioninfo"

//go:generate goversioninfo -platform-specific

var usage = `Windows Container layer utility

wclayer-v2 is a command line tool for manipulating Windows Container
storage layers. It can import and export layers from and to OCI format
layer tar files, create new writable layers, and mount and unmount
container images.

This utility requires Windows RS5 (Windows Server 2019) or later.`

func main() {
	app := cli.NewApp()
	app.Name = "wclayer"
	app.Commands = []cli.Command{
		createCommand,
		exportCommand,
		importCommand,
		makeBaseLayerCommand,
		mountCommand,
		removeCommand,
		unmountCommand,
	}
	app.Usage = usage

	if osversion.Build() < osversion.RS5 {
		fmt.Fprintf(os.Stderr, "This app requires build %d or later, you are on %d", osversion.RS5, osversion.Build())
		os.Exit(1)
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
