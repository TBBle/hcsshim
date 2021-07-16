package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim/computestorage"
	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/urfave/cli"
	"golang.org/x/sys/windows"
)

var mountCommand = cli.Command{
	Name:      "mount",
	Usage:     "activates a scratch, optionally mounted to provided target",
	ArgsUsage: "<scratch path> [target path]",
	Before:    appargs.Validate(appargs.NonEmptyString, appargs.Optional(appargs.String)),
	Flags: []cli.Flag{
		cli.StringSliceFlag{
			Name:  "layer, l",
			Usage: "paths to the parent layers for this layer",
		},
	},
	Action: func(cliContext *cli.Context) (err error) {
		path, err := filepath.Abs(cliContext.Args().Get(0))
		if err != nil {
			return err
		}

		sandboxVHDXPath := filepath.Join(path, containerSandboxVHDXName)

		if _, err := os.Stat(sandboxVHDXPath); err != nil {
			return fmt.Errorf("failed to Stat %q: %w", sandboxVHDXPath, err)
		}

		if err := vhd.AttachVhd(sandboxVHDXPath); err != nil {
			return fmt.Errorf("failed to Attach %q: %w", sandboxVHDXPath, err)
		}

		defer func() {
			if err != nil {
				_ = vhd.DetachVhd(sandboxVHDXPath)
			}
		}()

		/* TODO
		targetPath, err := filepath.Abs(cliContext.Args().Get(1))
		if err != nil {
			return err
		}
		*/

		ctx := context.Background()

		layerData, err := getLayerData(ctx, cliContext.StringSlice("layer"), true)
		if err != nil {
			return err
		}

		// TODO: I need to mount the sandbox somewhere first?

		err = computestorage.AttachLayerStorageFilter(ctx, path, *layerData)
		if err != nil {
			return err
		}
		defer func() {
			if err != nil {
				_ = computestorage.DetachLayerStorageFilter(ctx, path)
			}
		}()

		// Copied out of vhd.AttachVhd
		vhdHandle, err := vhd.OpenVirtualDisk(
			sandboxVHDXPath,
			vhd.VirtualDiskAccessNone,
			vhd.OpenVirtualDiskFlagCachedIO|vhd.OpenVirtualDiskFlagIgnoreRelativeParentLocator,
		)
		defer windows.CloseHandle(windows.Handle(vhdHandle))

		mountPath, err := computestorage.GetLayerVhdMountPath(ctx, windows.Handle(vhdHandle))
		if err != nil {
			return err
		}

		/* TODO:

		if cliContext.NArg() == 2 {
			if err = setVolumeMountPoint(targetPath, mountPath); err != nil {
				return err
			}
			_, err = fmt.Println(targetPath)
			return err
		}
		*/

		_, err = fmt.Println(mountPath)
		return err
	},
}

var unmountCommand = cli.Command{
	Name:      "unmount",
	Usage:     "deactivates a scratch, optionally unmounting",
	ArgsUsage: "<scratch path> [mounted path]",
	Before:    appargs.Validate(appargs.NonEmptyString, appargs.Optional(appargs.String)),
	Action: func(cliContext *cli.Context) (err error) {
		path, err := filepath.Abs(cliContext.Args().Get(0))
		if err != nil {
			return err
		}

		ctx := context.Background()

		/* TODO
		mountedPath, err := filepath.Abs(context.Args().Get(1))
		if err != nil {
			return err
		}

		if context.NArg() == 2 {
			if err = deleteVolumeMountPoint(mountedPath); err != nil {
				return err
			}
		}
		*/

		return computestorage.DetachLayerStorageFilter(ctx, path)
	},
}
