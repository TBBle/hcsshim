package main

import (
	"path/filepath"

	"github.com/Microsoft/hcsshim"
	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/urfave/cli"
)

var makeBaseLayerCommand = cli.Command{
	Name:      "makebaselayer",
	Usage:     "converts a directory containing 'Files/' into a base layer",
	ArgsUsage: "<layer path>",
	Before:    appargs.Validate(appargs.NonEmptyString),
	Action: func(context *cli.Context) error {
		path, err := filepath.Abs(context.Args().First())
		if err != nil {
			return err
		}

		// TODO
		// Per `createScratchLayer` in containerd
		// https://github.com/containerd/containerd/blob/main/snapshots/windows/windows.go#L428-L469
		// computestorage.SetupContainerBaseLayer(ctx, baseLayerPath, baseLayerPath/"blank-base.vhdx", baseLayerPath/"blank.vhdx", sizeGB=20);
		// creates those two VHDX files, and creating a scratch layer is simply copying "blank.vhdx"
		// as "sandbox.vhdx", and possibly expanding the sandbox size.
		// That wraps computestorage.FormatWritableLayerVhd and computestorage.SetupBaseOSLayer, as well
		// as the necessary VHD operations. (It's not actually a computestorage.dll API, it's a helper.)
		// An earlier version of that PR (https://github.com/containerd/containerd/pull/4912)
		// https://github.com/containerd/containerd/blob/d88f9a1ea898a789f478c20eeb642e59b9aa8327/snapshots/windows/windows.go
		// has longer `createUVMScratchLayer` which demonstrates `computestorage.SetupUtilityVMBaseLayer`
		// computestorage.SetupUtilityVMBaseLayer(ctx, baseLayerPath/"UtilityVM", baseLayerPath/"UtilityVM/SystemTemplateBase.vhdx", baseLayerPath/"UtilityVM/SystemTemplate.vhdx", sizeGB=10)
		return hcsshim.ConvertToBaseLayer(path)
	},
}
