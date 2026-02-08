package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/arumata/devback/internal/usecase"
)

// Adapter implements FileSystemAdapter using standard os and filepath packages
type Adapter struct {
	logger *slog.Logger
}

// New creates a new filesystem adapter.
func New(logger *slog.Logger) *Adapter {
	if logger == nil {
		panic("filesystem adapter requires logger")
	}
	return &Adapter{logger: logger}
}

// ReadFile reads file content
func (a *Adapter) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return os.ReadFile(path) // #nosec G304 - paths are controlled by usecase
}

// WriteFile writes content to file
func (a *Adapter) WriteFile(ctx context.Context, path string, data []byte, perm int) error {
	if perm < 0 || perm > 0o777 {
		perm = 0o644 // Default safe permissions
	}
	// #nosec G115 - perm is validated to be within safe range
	return os.WriteFile(path, data, fs.FileMode(perm))
}

// CreateDir creates directory with permissions
func (a *Adapter) CreateDir(ctx context.Context, path string, perm int) error {
	if perm < 0 || perm > 0o777 {
		perm = 0o755 // Default safe permissions
	}
	// #nosec G115 - perm is validated to be within safe range
	return os.MkdirAll(path, fs.FileMode(perm))
}

// RemoveAll removes directory and all contents
func (a *Adapter) RemoveAll(ctx context.Context, path string) error {
	return os.RemoveAll(path)
}

// Stat returns file info
func (a *Adapter) Stat(ctx context.Context, path string) (usecase.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &fileInfoWrapper{info}, nil
}

// Lstat returns file info without following symlinks
func (a *Adapter) Lstat(ctx context.Context, path string) (usecase.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	return &fileInfoWrapper{info}, nil
}

// Walk traverses directory tree
func (a *Adapter) Walk(ctx context.Context, root string, walkFn usecase.WalkFunc) error {
	return filepath.Walk(root, func(path string, info fs.FileInfo, err error) error {
		var fileInfo usecase.FileInfo
		if info != nil {
			fileInfo = &fileInfoWrapper{info}
		}
		return walkFn(path, fileInfo, err)
	})
}

// ReadDir lists directory entries
func (a *Adapter) ReadDir(ctx context.Context, path string) ([]usecase.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	result := make([]usecase.DirEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, &dirEntryWrapper{entry})
	}
	return result, nil
}

// Glob returns paths matching pattern
func (a *Adapter) Glob(ctx context.Context, pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

// CreateDirExclusive creates directory only if it does not exist
func (a *Adapter) CreateDirExclusive(ctx context.Context, path string, perm int) error {
	if perm < 0 || perm > 0o777 {
		perm = 0o755
	}
	// #nosec G115 - perm is validated to be within safe range
	return os.Mkdir(path, fs.FileMode(perm))
}

// Copy copies file from src to dst
func (a *Adapter) Copy(ctx context.Context, src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}

	srcFile, err := os.Open(src) // #nosec G304 - paths are controlled by usecase
	if err != nil {
		return err
	}
	defer func() {
		_ = srcFile.Close() // Ignore close error in defer
	}()

	// Get source file info for permissions
	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.Create(dst) // #nosec G304 - paths are controlled by usecase
	if err != nil {
		return err
	}
	defer func() {
		_ = dstFile.Close() // Ignore close error in defer
	}()

	// Copy content
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return err
	}

	// Set permissions
	return os.Chmod(dst, srcInfo.Mode())
}

// Move moves file from src to dst
func (a *Adapter) Move(ctx context.Context, src, dst string) error {
	return os.Rename(src, dst)
}

// Readlink reads symlink target
func (a *Adapter) Readlink(ctx context.Context, path string) (string, error) {
	return os.Readlink(path)
}

// Symlink creates symlink
func (a *Adapter) Symlink(ctx context.Context, target, path string) error {
	return os.Symlink(target, path)
}

// Chmod changes file mode
func (a *Adapter) Chmod(ctx context.Context, path string, perm int) error {
	if perm < 0 || perm > 0o777 {
		return fmt.Errorf("invalid permission bits: %o", perm)
	}
	return os.Chmod(path, fs.FileMode(perm))
}

// Chtimes changes access and modification times
func (a *Adapter) Chtimes(ctx context.Context, path string, atime, mtime time.Time) error {
	return os.Chtimes(path, atime, mtime)
}

// GetWorkingDir returns current working directory
func (a *Adapter) GetWorkingDir(ctx context.Context) (string, error) {
	return os.Getwd()
}

// Abs returns absolute path
func (a *Adapter) Abs(ctx context.Context, path string) (string, error) {
	return filepath.Abs(path)
}

// Join joins path elements
func (a *Adapter) Join(elements ...string) string {
	return filepath.Join(elements...)
}

// Base returns last element of path
func (a *Adapter) Base(path string) string {
	return filepath.Base(path)
}

// Dir returns directory of path
func (a *Adapter) Dir(path string) string {
	return filepath.Dir(path)
}

// Ext returns file extension
func (a *Adapter) Ext(path string) string {
	return filepath.Ext(path)
}

// IsAbs reports whether the path is absolute.
func (a *Adapter) IsAbs(path string) bool {
	return filepath.IsAbs(path)
}

// Rel returns a relative path.
func (a *Adapter) Rel(basepath, targpath string) (string, error) {
	return filepath.Rel(basepath, targpath)
}

// Clean returns the cleaned path.
func (a *Adapter) Clean(path string) string {
	return filepath.Clean(path)
}

// VolumeName returns the volume name of path.
func (a *Adapter) VolumeName(path string) string {
	return filepath.VolumeName(path)
}

// PathSeparator returns the OS-specific path separator.
func (a *Adapter) PathSeparator() byte {
	return os.PathSeparator
}

// IsNotExist reports whether err indicates that a path does not exist.
// Also covers syscall.ENOTDIR (path component is not a directory).
func (a *Adapter) IsNotExist(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR)
}

// IsExist reports whether err indicates that a path already exists.
func (a *Adapter) IsExist(err error) bool {
	return os.IsExist(err)
}

// IsPermission reports whether err indicates a permission error.
func (a *Adapter) IsPermission(err error) bool {
	return os.IsPermission(err) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)
}

// TempDir creates temporary directory
func (a *Adapter) TempDir(ctx context.Context, dir, prefix string) (string, error) {
	return os.MkdirTemp(dir, prefix)
}

// fileInfoWrapper wraps os.FileInfo to implement usecase.FileInfo
type fileInfoWrapper struct {
	fs.FileInfo
}

// Name returns the name of the file
func (w *fileInfoWrapper) Name() string {
	return w.FileInfo.Name()
}

// Size returns the size of the file
func (w *fileInfoWrapper) Size() int64 {
	return w.FileInfo.Size()
}

// Mode returns the file mode
func (w *fileInfoWrapper) Mode() int {
	return int(w.FileInfo.Mode())
}

// ModTime returns the modification time
func (w *fileInfoWrapper) ModTime() time.Time {
	return w.FileInfo.ModTime()
}

// IsDir returns true if the file is a directory
func (w *fileInfoWrapper) IsDir() bool {
	return w.FileInfo.IsDir()
}

// IsSymlink returns true if the file is a symbolic link
func (w *fileInfoWrapper) IsSymlink() bool {
	return w.FileInfo.Mode()&os.ModeSymlink != 0
}

// IsRegular returns true if the file is a regular file
func (w *fileInfoWrapper) IsRegular() bool {
	return w.FileInfo.Mode().IsRegular()
}

// Sys returns underlying data source
func (w *fileInfoWrapper) Sys() interface{} {
	return w.FileInfo.Sys()
}

type dirEntryWrapper struct {
	fs.DirEntry
}

func (w *dirEntryWrapper) Name() string {
	return w.DirEntry.Name()
}

func (w *dirEntryWrapper) IsDir() bool {
	return w.DirEntry.IsDir()
}
