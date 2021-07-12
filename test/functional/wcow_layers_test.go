// +build windows

package functional

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/windows"

	layerspkg "github.com/Microsoft/hcsshim/internal/layers"
	"github.com/Microsoft/hcsshim/internal/oc"
	"github.com/Microsoft/hcsshim/internal/wclayer"
	"github.com/Microsoft/hcsshim/pkg/ociwclayer"
	testutilities "github.com/Microsoft/hcsshim/test/functional/utilities"
)

// Initially, just an attempt to recreate the failure of the 128LayersMount test
// (and similar failures) in the containerd Snapshot Testsuite.
// https://github.com/containerd/containerd/pull/4419#issuecomment-762830024

// The rough logic of 128layers is:
// Create a base layer:
/*
	fstest.CreateFile("/bottom", []byte("way at the bottom\n"), 0777),
	fstest.CreateFile("/overwriteme", []byte("FIRST!\n"), 0777),
	fstest.CreateDir("/addhere", 0755),
	fstest.CreateDir("/onlyme", 0755),
	fstest.CreateFile("/onlyme/bottom", []byte("bye!\n"), 0777),
*/
// Then for 1..127, the layer is mounted, the below done, then the layer is
// unmounted and turned into a base layer for the next iteration.
/*
	fstest.CreateFile("/overwriteme", []byte(fmt.Sprintf("%d WAS HERE!\n", i)), 0777),
	fstest.CreateFile(fmt.Sprintf("/addhere/file-%d", i), []byte("same\n"), 0755),
	fstest.RemoveAll("/onlyme"),
	fstest.CreateDir("/onlyme", 0755),
	fstest.CreateFile(fmt.Sprintf("/onlyme/file-%d", i), []byte("only me!\n"), 0777),
*/

func createFile(t *testing.T, root, path string, data []byte) {
	targetPath := filepath.Join(root, path)

	if file, err := os.Create(targetPath); err != nil {
		t.Fatal(err)
	} else {
		if _, err = file.Write(data); err != nil {
			t.Fatal(err)
		}

		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func createDir(t *testing.T, root, path string) {
	targetPath := filepath.Join(root, path)

	if err := os.MkdirAll(targetPath, 0); err != nil {
		t.Fatal(err)
	}
}

func removeAll(t *testing.T, root, path string) {
	targetPath := filepath.Join(root, path)

	if err := os.RemoveAll(targetPath); err != nil {
		t.Fatal(err)
	}
}

func mountLayer(t *testing.T, layerStack []string) string {
	volumePath, err := layerspkg.MountContainerLayers(context.Background(), layerStack, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return volumePath
}

func unmountLayer(t *testing.T, layerStack []string) {
	if err := layerspkg.UnmountContainerLayers(context.Background(), layerStack, "", "", nil, layerspkg.UnmountOperationAll); err != nil {
		t.Fatal(err)
	}
}

func createScratch(t *testing.T, layerStack []string) {
	path := layerStack[len(layerStack)-1]
	parentLayerPaths := layerStack[:len(layerStack)-1]

	if err := wclayer.CreateScratchLayer(context.Background(), path, parentLayerPaths); err != nil {
		t.Fatal(err)
	}

}

// Mount volumePath (in format '\\?\Volume{GUID}' at targetPath.
// https://docs.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-setvolumemountpointw
func setVolumeMountPoint(targetPath string, volumePath string) error {
	if !strings.HasPrefix(volumePath, "\\\\?\\Volume{") {
		return errors.Errorf("unable to mount non-volume path %s", volumePath)
	}

	// Both must end in a backslash
	slashedTarget := filepath.Clean(targetPath) + string(filepath.Separator)
	slashedVolume := volumePath + string(filepath.Separator)

	targetP, err := windows.UTF16PtrFromString(slashedTarget)
	if err != nil {
		return errors.Wrapf(err, "unable to utf16-ise %s", slashedTarget)
	}

	volumeP, err := windows.UTF16PtrFromString(slashedVolume)
	if err != nil {
		return errors.Wrapf(err, "unable to utf16-ise %s", slashedVolume)
	}

	if err := windows.SetVolumeMountPoint(targetP, volumeP); err != nil {
		return errors.Wrapf(err, "failed calling SetVolumeMount('%s', '%s')", slashedTarget, slashedVolume)
	}

	return nil
}

// Remove the volume mount at targetPath
// https://docs.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-deletevolumemountpointa
func deleteVolumeMountPoint(targetPath string) error {
	// Must end in a backslash
	slashedTarget := filepath.Clean(targetPath) + string(filepath.Separator)

	targetP, err := windows.UTF16PtrFromString(slashedTarget)
	if err != nil {
		return errors.Wrapf(err, "unable to utf16-ise %s", slashedTarget)
	}

	if err := windows.DeleteVolumeMountPoint(targetP); err != nil {
		return errors.Wrapf(err, "failed calling DeleteVolumeMountPoint('%s')", slashedTarget)
	}

	return nil
}

// "Turn a scratch layer into a RO layer" from my containerd PR.
// I feel like this is a layer function that hcsshim could export...
// We could possibly avoid going via the tar stream, on consideration.
func restreamLayer(t *testing.T, layerFolders []string) {
	ctx := context.Background()

	path := layerFolders[len(layerFolders)-1]
	parentLayerPaths := layerFolders[:len(layerFolders)-1]

	reader, writer := io.Pipe()

	go func() {
		err := ociwclayer.ExportLayerToTar(ctx, writer, path, parentLayerPaths)
		writer.CloseWithError(err)
	}()

	if _, err := ociwclayer.ImportLayerFromTar(ctx, reader, path, parentLayerPaths); err != nil {
		t.Fatalf("%s: %v", path, err)
	}

	if _, err := io.Copy(ioutil.Discard, reader); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func reverse(a []string) {
	for i := len(a)/2 - 1; i >= 0; i-- {
		opp := len(a) - 1 - i
		a[i], a[opp] = a[opp], a[i]
	}
}

func Test128Layers(t *testing.T) {
	if testing.Verbose() {
		trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
		trace.RegisterExporter(&oc.LogrusExporter{})
	}

	for i := 1; i <= 10; i++ {
		t.Run(fmt.Sprintf("test128Layers_%02d", i), test128Layers)
	}
}

func test128Layers(t *testing.T) {
	t.Parallel()

	// Get temp dir, and create base-dir with Files/ directory
	tempDir := testutilities.CreateTempDir(t)
	t.Cleanup(func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Fatal(err)
		}
	})

	baseDir := filepath.Join(tempDir, "layer-1")

	if err := os.MkdirAll(filepath.Join(baseDir, "Files"), 0); err != nil {
		t.Fatal(err)
	}

	// Apply the above filesystem changes
	// createDir(t, filepath.Join(baseDir, "Files"), "wcow_workaround")
	// createFile(t, filepath.Join(baseDir, "Files"), filepath.Join("wcow_workaround", "bottom"), []byte("way at the bottom\n"))
	// createFile(t, filepath.Join(baseDir, "Files"), filepath.Join("wcow_workaround", "overwriteme"), []byte("FIRST!\n"))
	// createDir(t, filepath.Join(baseDir, "Files"), filepath.Join("wcow_workaround", "addhere"))
	// createDir(t, filepath.Join(baseDir, "Files"), filepath.Join("wcow_workaround", "onlyme"))
	// createFile(t, filepath.Join(baseDir, "Files"), filepath.Join("wcow_workaround", "onlyme", "bottom"), []byte("bye!\n"))

	// Turn it into a Base Layer
	if err := wclayer.ConvertToBaseLayer(context.Background(), baseDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := wclayer.DestroyLayer(context.Background(), baseDir); err != nil {
			t.Fatal(err)
		}
	})

	layers := []string{baseDir}

	// Now loop:
	for i := 2; i <= 128; i++ {
		scratchDir := filepath.Join(tempDir, fmt.Sprintf("layer-%d", i))

		// Mount a scratch on our list
		layers = append(layers, scratchDir)
		reverse(layers[:len(layers)-1])
		createScratch(t, layers)
		t.Cleanup(func() {
			if err := wclayer.DestroyLayer(context.Background(), scratchDir); err != nil {
				t.Fatal(err)
			}
		})

		volumePath := mountLayer(t, layers)

		mountPath := filepath.Join(tempDir, fmt.Sprintf("mount-%d", i))
		if err := os.MkdirAll(mountPath, 0); err != nil {
			t.Fatal(err)
		}

		if err := setVolumeMountPoint(mountPath, volumePath); err != nil {
			t.Fatal(err)
		}

		if err := ioutil.WriteFile(mountPath+":containerd.io-source", []byte(volumePath), 0666); err != nil {
			t.Fatal(err)
		}

		// Perform the changes
		// createFile(t, mountPath, filepath.Join("wcow_workaround", "overwriteme"), []byte(fmt.Sprintf("%d WAS HERE!\n", i)))
		// createFile(t, mountPath, filepath.Join("wcow_workaround", "addhere", fmt.Sprintf("file-%d", i)), []byte("same\n"))
		// removeAll(t, mountPath, filepath.Join("wcow_workaround", "onlyme"))
		// createDir(t, mountPath, filepath.Join("wcow_workaround", "onlyme"))
		// createFile(t, mountPath, filepath.Join("wcow_workaround", "onlyme", fmt.Sprintf("file-%d", i)), []byte("only me!\n"))

		// for j := 1; j <= i; j++ {
		// 	addedFilePath := filepath.Join(mountPath, "wcow_workaround", "addhere", fmt.Sprintf("file-%d", j))
		// 	content, err := ioutil.ReadFile(addedFilePath)
		// 	if err != nil {
		// 		t.Fatal(err)
		// 	}
		// 	if bytes.Compare(content, []byte("same\n")) != 0 {
		// 		t.Fatalf("Failed to read back %s", addedFilePath)
		// 	}
		// }

		// Unmount the scratch
		if err := deleteVolumeMountPoint(mountPath); err != nil {
			t.Fatal(err)
		}

		if err := os.Remove(mountPath); err != nil {
			t.Fatal(err)
		}

		unmountLayer(t, layers)

		// Restream it as a read-only layer
		restreamLayer(t, layers)
		reverse(layers[:len(layers)-1])
	}

}

// TestLotsOfParallelMounts tests the theory that something in HCS is incorrectly
// mutexed, and will generate spurious failures in the presence of lots of parallel
// mount activity.
func TestLotsOfParallelMounts(t *testing.T) {
	if testing.Verbose() {
		trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
		trace.RegisterExporter(&oc.LogrusExporter{})
	}

	// Get temp dir, and create base-dir with Files/ directory
	tempDir := testutilities.CreateTempDir(t)
	t.Cleanup(func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Fatal(err)
		}
	})

	baseDir := filepath.Join(tempDir, "layer-0")

	if err := os.MkdirAll(filepath.Join(baseDir, "Files"), 0); err != nil {
		t.Fatal(err)
	}

	// Turn it into a Base Layer
	if err := wclayer.ConvertToBaseLayer(context.Background(), baseDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := wclayer.DestroyLayer(context.Background(), baseDir); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("1", makeTestAMountALot(baseDir))
	t.Run("2", makeTestAMountALot(baseDir))
	t.Run("3", makeTestAMountALot(baseDir))
	t.Run("4", makeTestAMountALot(baseDir))
	t.Run("5", makeTestAMountALot(baseDir))
	t.Run("6", makeTestAMountALot(baseDir))
	t.Run("7", makeTestAMountALot(baseDir))
	t.Run("8", makeTestAMountALot(baseDir))
}

// testAMountALot
func makeTestAMountALot(baseDir string) func(t *testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		tempDir := testutilities.CreateTempDir(t)
		t.Cleanup(func() {
			if err := os.RemoveAll(tempDir); err != nil {
				t.Fatal(err)
			}
		})

		// Now mount and unmount it
		for i := 1; i <= 127; i++ {
			scratchDir := filepath.Join(tempDir, fmt.Sprintf("layer-%d", i))

			// Create a scratch on our base dir
			layers := []string{baseDir, scratchDir}

			createScratch(t, layers)
			mountLayer(t, layers)
			unmountLayer(t, layers)
			restreamLayer(t, layers)
			if err := wclayer.DestroyLayer(context.Background(), scratchDir); err != nil {
				t.Fatal(err)
			}
		}
	}
}
