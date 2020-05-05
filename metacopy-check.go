// +build linux

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/containers/storage/pkg/ioutils"
	"github.com/containers/storage/pkg/mount"
	"github.com/containers/storage/pkg/system"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"k8s.io/klog"
)

// doesMetacopy checks if the filesystem is going to optimize changes to
// metadata by using nodes marked with an "overlay.metacopy" attribute to avoid
// copying up a file from a lower layer unless/until its contents are being
// modified
func doesMetacopy(d, mountOpts string) (bool, error) {
	klog.Info("creating temporary directory")
	td, err := ioutil.TempDir(d, "metacopy-check")
	if err != nil {
		return false, err
	}
	defer func() {
		klog.Info("removing temporary directory")
		if err := os.RemoveAll(td); err != nil {
			klog.Warningf("Failed to remove check directory %v: %v", td, err)
		}
	}()

	// Make directories l1, l2, work, merged
	klog.Info("creating lower layer")
	if err := os.MkdirAll(filepath.Join(td, "l1"), 0755); err != nil {
		return false, err
	}
	klog.Info("creating file in lower layer")
	if err := ioutils.AtomicWriteFile(filepath.Join(td, "l1", "f"), []byte{0xff}, 0700); err != nil {
		return false, err
	}
	klog.Info("creating upper layer")
	if err := os.MkdirAll(filepath.Join(td, "l2"), 0755); err != nil {
		return false, err
	}
	klog.Info("creating work directory")
	if err := os.Mkdir(filepath.Join(td, "work"), 0755); err != nil {
		return false, err
	}
	klog.Info("creating mount point")
	if err := os.Mkdir(filepath.Join(td, "merged"), 0755); err != nil {
		return false, err
	}
	// Mount using the mandatory options and configured options
	klog.Info("parsing options")
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", path.Join(td, "l1"), path.Join(td, "l2"), path.Join(td, "work"))
	flags, data := mount.ParseOptions(mountOpts)
	if data != "" {
		opts = fmt.Sprintf("%s,%s", opts, data)
	}
	klog.Info("mounting layers")
	if err := unix.Mount("overlay", filepath.Join(td, "merged"), "overlay", uintptr(flags), opts); err != nil {
		return false, errors.Wrap(err, "failed to mount overlay for metacopy check")
	}
	defer func() {
		klog.Info("unmounting layers")
		if err := unix.Unmount(filepath.Join(td, "merged"), 0); err != nil {
			klog.Warningf("Failed to unmount check directory %v: %v", filepath.Join(td, "merged"), err)
		}
	}()
	// Make a change that only impacts the inode, and check if the pulled-up copy is marked
	// as a metadata-only copy
	klog.Info("modifying test file permissions")
	if err := os.Chmod(filepath.Join(td, "merged", "f"), 0600); err != nil {
		return false, errors.Wrap(err, "error changing permissions on file for metacopy check")
	}
	klog.Info("reading attributes of test file")
	metacopy, err := system.Lgetxattr(filepath.Join(td, "l2", "f"), "trusted.overlay.metacopy")
	if err != nil {
		return false, errors.Wrap(err, "metacopy flag was not set on file in upper layer")
	}
	return metacopy != nil, nil
}

func main() {
	dirs := []string{"."}
	if len(os.Args) > 1 {
		dirs = os.Args[1:]
	}
	for _, dir := range dirs {
		does, err := doesMetacopy(dir, "")
		if err != nil {
			klog.Fatalf("%s: %v", dir, err)
		}
		klog.Infof("%s: %v\n\n", dir, does)
	}
}
