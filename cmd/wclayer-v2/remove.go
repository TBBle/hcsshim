package main

import (
	"context"
	"path/filepath"

	winio "github.com/Microsoft/go-winio"
	"github.com/Microsoft/hcsshim/computestorage"
	"github.com/Microsoft/hcsshim/internal/appargs"

	"github.com/urfave/cli"
)

var removeCommand = cli.Command{
	Name:      "remove",
	Usage:     "permanently removes a layer directory in its entirety",
	ArgsUsage: "<layer path>",
	Before:    appargs.Validate(appargs.NonEmptyString),
	Action: func(cliContext *cli.Context) (err error) {
		path, err := filepath.Abs(cliContext.Args().First())
		if err != nil {
			return err
		}

		err = winio.EnableProcessPrivileges([]string{winio.SeBackupPrivilege, winio.SeRestorePrivilege})
		if err != nil {
			return err
		}

		return computestorage.DestroyLayer(context.Background(), path)
	},
}
