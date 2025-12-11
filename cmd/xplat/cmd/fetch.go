package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"
	"github.com/spf13/cobra"
)

// FetchCmd downloads files with optional extraction
var FetchCmd = &cobra.Command{
	Use:   "fetch <url>",
	Short: "Download files with optional archive extraction",
	Long: `Download a file from a URL, optionally extracting if it's an archive.

Examples:
  # Simple download
  xplat fetch https://example.com/file.txt --output ./downloads

  # Download and extract archive
  xplat fetch --extract https://github.com/cli/cli/releases/download/v2.83.1/gh_2.83.1_macOS_arm64.zip --output ~/.local/bin

  # Extract with path manipulation
  xplat fetch --extract https://example.com/release.tar.gz --output ./bin --strip 2 --include "*/bin/*"

Flags:
  --output DIR    Output directory (default: current directory)
  --extract       Extract archive after downloading
  --strip N       Remove N leading path components (with --extract)
  --include GLOB  Only extract files matching pattern (with --extract)`,
	Args: cobra.ExactArgs(1),
	RunE: runFetch,
}

var (
	fetchOutput  string
	fetchExtract bool
	fetchStrip   int
	fetchInclude string
)

func init() {
	FetchCmd.Flags().StringVarP(&fetchOutput, "output", "o", ".", "Output directory")
	FetchCmd.Flags().BoolVarP(&fetchExtract, "extract", "x", false, "Extract archive after downloading")
	FetchCmd.Flags().IntVar(&fetchStrip, "strip", 0, "Remove N leading path components (with --extract)")
	FetchCmd.Flags().StringVar(&fetchInclude, "include", "", "Only extract files matching glob pattern (with --extract)")
}

func runFetch(cmd *cobra.Command, args []string) error {
	url := args[0]

	// Create output directory
	if err := os.MkdirAll(fetchOutput, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	// Download
	fmt.Printf("Downloading %s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Get filename from URL
	urlPath := strings.TrimSuffix(url, "/")
	filename := filepath.Base(urlPath)

	if !fetchExtract {
		// Simple download - save to file
		destPath := filepath.Join(fetchOutput, filename)
		return downloadToFile(resp.Body, destPath)
	}

	// Download and extract
	return downloadAndExtract(resp.Body, url, filename)
}

func downloadToFile(reader io.Reader, destPath string) error {
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("cannot create file: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, reader)
	if err != nil {
		return fmt.Errorf("download incomplete: %w", err)
	}

	fmt.Printf("Downloaded %d bytes to %s\n", written, destPath)
	return nil
}

func downloadAndExtract(reader io.Reader, url, filename string) error {
	// Create temp file to store download (needed for seeking archives like zip)
	tmpFile, err := os.CreateTemp("", "xplat-fetch-*-"+filename)
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Download to temp file
	written, err := io.Copy(tmpFile, reader)
	if err != nil {
		return fmt.Errorf("download incomplete: %w", err)
	}
	fmt.Printf("Downloaded %d bytes\n", written)

	// Seek back to start
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("cannot seek: %w", err)
	}

	// Identify archive format
	ctx := context.Background()
	format, input, err := archives.Identify(ctx, filename, tmpFile)
	if err != nil {
		return fmt.Errorf("cannot identify archive format: %w", err)
	}

	ex, ok := format.(archives.Extractor)
	if !ok {
		return fmt.Errorf("format %s does not support extraction", format.MediaType())
	}

	// Seek back again for extraction (Identify may have consumed some bytes)
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("cannot seek: %w", err)
	}

	// For zip-like formats, use the file directly
	var extractReader io.Reader = input
	if _, seekable := input.(io.Seeker); !seekable {
		extractReader = tmpFile
	}

	return extractFetchedArchive(ctx, ex, extractReader, written, filename, fetchOutput)
}

func extractFetchedArchive(ctx context.Context, ex archives.Extractor, reader io.Reader, size int64, name string, destDir string) error {
	extractedCount := 0

	handler := func(ctx context.Context, f archives.FileInfo) error {
		path := f.NameInArchive

		// Apply --strip
		if fetchStrip > 0 {
			parts := strings.Split(path, "/")
			if len(parts) <= fetchStrip {
				return nil
			}
			path = strings.Join(parts[fetchStrip:], "/")
		}

		if path == "" || path == "." {
			return nil
		}

		// Apply --include filter
		if fetchInclude != "" {
			matched := matchGlob(fetchInclude, f.NameInArchive)
			if !matched {
				matched, _ = filepath.Match(fetchInclude, filepath.Base(f.NameInArchive))
			}
			if !matched {
				return nil
			}
		}

		destPath := filepath.Join(destDir, path)

		if f.IsDir() {
			return os.MkdirAll(destPath, f.Mode())
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("cannot create directory: %w", err)
		}

		src, err := f.Open()
		if err != nil {
			return fmt.Errorf("cannot open file in archive: %w", err)
		}
		defer src.Close()

		dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("cannot create file: %w", err)
		}
		defer dst.Close()

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
