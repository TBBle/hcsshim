package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/internal/appargs"
	"github.com/urfave/cli"
)

var createCommand = cli.Command{
	Name:  "create",
	Usage: "creates a new writable container layer",
	Flags: []cli.Flag{
		cli.StringSliceFlag{
			Name:  "layer, l",
			Usage: "paths to the read-only parent layers",
		},
	},
	ArgsUsage: "<layer path>",
	Before:    appargs.Validate(appargs.NonEmptyString),
	Action: func(cliContext *cli.Context) error {
		path, err := filepath.Abs(cliContext.Args().First())
		if err != nil {
			return err
		}

		ctx := context.Background()

		if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
			if err == nil {
				return fmt.Errorf("cannot create layer %q, something already exists", path)
			}
			return fmt.Errorf("failed to Stat %q: %w", path, err)
		}

		// Technically, we only need the base layer path, but this is a good sanity check.
		layerData, err := getLayerData(ctx, cliContext.StringSlice("layer"), true)
		if err != nil {
			return err
		}

		baseLayerPath := layerData.Layers[len(layerData.Layers)-1].Path
		templateVHDXPath := filepath.Join(baseLayerPath, containerSandboxTemplateVHDXName)
		template, err := os.Open(templateVHDXPath)
		if err != nil {
			return fmt.Errorf("failed to Open %q for reading: %w", templateVHDXPath, err)
		}
		defer template.Close()

		if err := os.MkdirAll(path, 0777); err != nil {
			return fmt.Errorf("failed to create %q: %w", path, err)
		}

		sandboxVHDXPath := filepath.Join(path, containerSandboxVHDXName)
		sandbox, err := os.Create(sandboxVHDXPath)
		if err != nil {
			return fmt.Errorf("failed to Create %q for writing: %w", sandboxVHDXPath, err)
		}
		defer sandbox.Close()

		if _, err := io.Copy(sandbox, template); err != nil {
			return fmt.Errorf("failed to Copy %q to %q: %w", templateVHDXPath, sandboxVHDXPath, err)
		}

		return nil
	},
}
