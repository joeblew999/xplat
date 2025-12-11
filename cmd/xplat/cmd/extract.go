package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"
	"github.com/spf13/cobra"
)

// ExtractCmd extracts archives
var ExtractCmd = &cobra.Command{
	Use:   "extract <archive> [destination]",
	Short: "Extract archives (zip, tar.gz, tar.bz2, tar.xz, 7z, rar)",
	Long: `Extract archive files with automatic format detection.

Supported formats: zip, tar, tar.gz, tar.bz2, tar.xz, tar.zst, 7z, rar

Examples:
  xplat extract archive.zip
  xplat extract archive.tar.gz ./dest
  xplat extract --strip 1 gh_2.83.1_macOS_arm64.zip
  xplat extract --include "*/bin/*" --strip 2 gh.zip ./bin
  xplat extract --list archive.zip

Flags:
  --strip N       Remove N leading path components from extracted files
  --include GLOB  Only extract files matching glob pattern
  --list          List archive contents without extracting`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runExtract,
}

var (
	extractStrip   int
	extractInclude string
	extractList    bool
)

func init() {
	ExtractCmd.Flags().IntVar(&extractStrip, "strip", 0, "Remove N leading path components")
	ExtractCmd.Flags().StringVar(&extractInclude, "include", "", "Only extract files matching glob pattern")
	ExtractCmd.Flags().BoolVar(&extractList, "list", false, "List contents without extracting")
}

func runExtract(cmd *cobra.Command, args []string) error {
	archivePath := args[0]

	// Determine destination
	destDir := "."
	if len(args) > 1 {
		destDir = args[1]
	}

	// Open the archive file
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("cannot open archive: %w", err)
	}
	defer f.Close()

	// Get file info for size
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat archive: %w", err)
	}

	// Identify the archive format
	ctx := context.Background()
	format, input, err := archives.Identify(ctx, archivePath, f)
	if err != nil {
		return fmt.Errorf("cannot identify archive format: %w", err)
	}

	// Check if it's an extractor
	ex, ok := format.(archives.Extractor)
	if !ok {
		return fmt.Errorf("format %s does not support extraction", format.MediaType())
	}

	// For seekable input, we need to handle the reader properly
	// The Identify function may have consumed some bytes, so we need
	// to use the returned input which handles this
	var reader io.Reader = input

	// If the format supports seeking (like zip), use the file directly
	if _, ok := format.(archives.Decompressor); !ok {
		// For zip-like formats that need random access, seek back to start
		f.Seek(0, io.SeekStart)
		reader = f
	}

	// List mode
	if extractList {
		return listArchive(ctx, ex, reader, info.Size(), archivePath)
	}

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("cannot create destination: %w", err)
	}

	// Extract
	return extractArchive(ctx, ex, reader, info.Size(), archivePath, destDir)
}

func listArchive(ctx context.Context, ex archives.Extractor, reader io.Reader, size int64, name string) error {
	handler := func(ctx context.Context, f archives.FileInfo) error {
		path := f.NameInArchive
		if f.IsDir() {
			path += "/"
		}
		fmt.Println(path)
		return nil
	}

	return ex.Extract(ctx, reader, handler)
}

func extractArchive(ctx context.Context, ex archives.Extractor, reader io.Reader, size int64, name string, destDir string) error {
	extractedCount := 0

	handler := func(ctx context.Context, f archives.FileInfo) error {
		// Get the path within the archive
		path := f.NameInArchive

		// Apply --strip
		if extractStrip > 0 {
			parts := strings.Split(path, "/")
			if len(parts) <= extractStrip {
				return nil // Skip this file - not enough path components
			}
			path = strings.Join(parts[extractStrip:], "/")
		}

		// Skip empty paths after stripping
		if path == "" || path == "." {
			return nil
		}

		// Apply --include filter
		if extractInclude != "" {
			matched, err := filepath.Match(extractInclude, f.NameInArchive)
			if err != nil {
				return fmt.Errorf("invalid include pattern: %w", err)
			}
			// Also try matching just the filename
			if !matched {
				matched, _ = filepath.Match(extractInclude, filepath.Base(f.NameInArchive))
			}
			// Try matching with wildcards for path segments
			if !matched {
				matched = matchGlob(extractInclude, f.NameInArchive)
			}
			if !matched {
				return nil
			}
		}

		destPath := filepath.Join(destDir, path)

		// Handle directories
		if f.IsDir() {
			return os.MkdirAll(destPath, f.Mode())
		}

		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("cannot create directory: %w", err)
		}

		// Open source
		src, err := f.Open()
		if err != nil {
			return fmt.Errorf("cannot open file in archive: %w", err)
		}
		defer src.Close()

		// Create destination file
		dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("cannot create file: %w", err)
		}
		defer dst.Close()

		// Copy content
		if _, err := io.Copy(dst, src); err != nil {
			return fmt.Errorf("cannot write file: %w", err)
		}

		extractedCount++
		return nil
	}

	if err := ex.Extract(ctx, reader, handler); err != nil {
		return err
	}

	fmt.Printf("Extracted %d files to %s\n", extractedCount, destDir)
	return nil
}

// matchGlob provides more flexible glob matching for paths
// Supports patterns like "*/bin/*" matching "gh_2.83.1/bin/gh"
func matchGlob(pattern, path string) bool {
	// Split both into parts
	patternParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")

	return matchParts(patternParts, pathParts)
}

func matchParts(pattern, path []string) bool {
	if len(pattern) == 0 && len(path) == 0 {
		return true
	}
	if len(pattern) == 0 {
		return false
	}

	// Handle ** (match zero or more directories)
	if pattern[0] == "**" {
		// Try matching ** as zero directories
		if matchParts(pattern[1:], path) {
			return true
		}
		// Try matching ** as one or more directories
		if len(path) > 0 && matchParts(pattern, path[1:]) {
			return true
		}
		return false
	}

	if len(path) == 0 {
		return false
	}

	// Handle * (match single component)
	if pattern[0] == "*" {
		return matchParts(pattern[1:], path[1:])
	}

	// Handle regular glob matching for current component
	matched, err := filepath.Match(pattern[0], path[0])
	if err != nil || !matched {
		return false
	}

	return matchParts(pattern[1:], path[1:])
}
