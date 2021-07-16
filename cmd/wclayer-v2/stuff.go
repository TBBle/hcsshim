package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/hcsshim/computestorage"
	hcsschema "github.com/Microsoft/hcsshim/internal/hcs/schema2"
	"github.com/Microsoft/hcsshim/internal/schemaversion"
	"github.com/Microsoft/hcsshim/internal/wclayer"
)

// Some filenames that the RS5 computestorage APIs let the caller choose.
// These are chosen to match the hard-coded names used in the RS1 layer APIs
// so that wclayer and wclayer-v2 can interoperate on the same layers.
const (
	containerSandboxDefaultSizeInGB  = 20
	containerSandboxBaseVHDXName     = "blank-base.vhdx"
	containerSandboxTemplateVHDXName = "blank.vhdx"
	containerSandboxVHDXName         = "sandbox.vhdx"

	uvmSandboxDefaultSizeInGB  = 10
	uvmSandboxBaseVHDXName     = "SystemTemplateBase.vhdx"
	uvmSandboxTemplateVHDXName = "SystemTemplate.vhdx"
	uvmSandboxVHDXName         = "sandbox.vhdx"
)

func normalizeLayers(il []string, needOne bool) ([]string, error) {
	if needOne && len(il) == 0 {
		return nil, errors.New("at least one read-only layer must be specified")
	}
	ol := make([]string, len(il))
	for i := range il {
		var err error
		ol[i], err = filepath.Abs(il[i])
		if err != nil {
			return nil, err
		}
	}
	return ol, nil
}

// Convert the list of layers into a computestorage.LayerData, and make sure they all exist, and contain a Files/ directory.
// TODO: Also confirm that the base-most layer is correctly set up:
// * Has containerSandboxBaseVHDXName and containerSandboxTemplateVHDXName
// * If UtilityVM/ directory exists, ensures uvmSandboxBaseVHDXName and uvmSandboxTemplateVHDXName exist
func getLayerData(ctx context.Context, il []string, needOne bool) (*computestorage.LayerData, error) {
	if needOne && len(il) == 0 {
		return nil, errors.New("at least one read-only layer must be specified")
	}

	result := &computestorage.LayerData{
		SchemaVersion: *schemaversion.SchemaV21(),
		Layers:        make([]hcsschema.Layer, len(il)),
	}

	for i := 0; i < len(il); i++ {
		fullPath, err := filepath.Abs(il[i])
		if err != nil {
			return nil, fmt.Errorf("failed to absolutify %q: %w", il[i], err)
		}

		if st, err := os.Stat(fullPath); err != nil || !st.IsDir() {
			if err == nil {
				return nil, fmt.Errorf("%q is not a directory, and cannot be used as a parent layer", fullPath)
			}
			return nil, fmt.Errorf("failed to Stat %q: %w", fullPath, err)
		}

		filesDir := filepath.Join(fullPath, "Files")

		if st, err := os.Stat(filesDir); err != nil || !st.IsDir() {
			if err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to Stat %q: %w", filesDir, err)
			}
			return nil, fmt.Errorf("%q is not a directory, cannot use %q as a parent layer", filesDir, fullPath)
		}

		// TODO: UtiityVM checks? @TBBle has forgotten if intermediate layers can have
		// a Utility VM, and if they can, what does that mean?

		//lint:ignore SA9003 It's not empty, there's a TODO comment there.
		if i == len(il)-1 {
			// TODO: Base layer checks.
		}

		// TODO: One line, three different notes
		// (a) This is a v1 API... I think we're supposed to manage this ourselves in computestorage land?
		// (b) The calculation may have assumptions we are missing, see https://github.com/microsoft/hcsshim/issues/1072
		// (c) We may want to use the v1 calculation (wclayer.LayerId) for wclayer compatibility, pending (b)
		guid, err := wclayer.NameToGuid(ctx, fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup Guid for %q: %w", fullPath, err)
		}

		result.Layers[i] = hcsschema.Layer{
			Id:   guid.String(),
			Path: fullPath,
			// This is an enum in the spec, but code-gen has missed it?
			PathType: "AbsolutePath",
		}
	}

	return result, nil
}
