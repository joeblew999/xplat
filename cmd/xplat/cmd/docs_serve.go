package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/joeblew999/xplat/internal/config"
	"github.com/joeblew999/xplat/internal/templates"
	"github.com/spf13/cobra"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/toc"
)

var servePort string
var buildOutput string
var docsVersion = "dev" // Set via SetDocsVersion

// SetDocsVersion sets the version shown in docs
func SetDocsVersion(v string) {
	docsVersion = v
}

// DocsServeCmd serves documentation locally with same styling as GitHub Pages
var DocsServeCmd = &cobra.Command{
	Use:   "docs",
	Short: "Documentation commands (serve, build)",
	Long: `Documentation commands for local preview and static site generation.

Subcommands:
  serve  - Start local server for live preview
  build  - Generate static HTML for GitHub Pages

Examples:
  xplat docs serve           # Live preview on port 8764
  xplat docs build           # Generate _site/ for GitHub Pages
  xplat docs build -o dist   # Custom output directory`,
}

var docsServeSubCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve docs locally (live preview)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return serveDocs(servePort)
	},
}

var docsBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build static HTML for GitHub Pages",
	Long: `Generate static HTML files to _site/ directory.

This creates the exact same output that GitHub Pages will serve.
Use this for deployment or to verify the build locally.

Examples:
  xplat docs build                    # Output to _site/
  xplat docs build -o dist            # Output to dist/
  xplat docs build --base /my-repo    # Set base path for GitHub Pages project site`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return buildDocs(buildOutput, buildBasePath)
	},
}

var buildBasePath string

func init() {
	docsServeSubCmd.Flags().StringVarP(&servePort, "port", "p", config.DefaultDocsPort, "Port to serve on")
	docsBuildCmd.Flags().StringVarP(&buildOutput, "output", "o", "_site", "Output directory")
	docsBuildCmd.Flags().StringVar(&buildBasePath, "base", "", "Base URL path for GitHub Pages (e.g., /my-repo)")

	DocsServeCmd.AddCommand(docsServeSubCmd)
	DocsServeCmd.AddCommand(docsBuildCmd)
}

// newMarkdown creates a configured goldmark instance with all extensions
func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,            // Tables, Strikethrough, Linkify, TaskList
			extension.DefinitionList, // Definition lists (term: definition)
			extension.Footnote,       // Footnotes [^1]
			extension.Typographer,    // Smart quotes, dashes, ellipsis
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // Generates <h1 id="my-heading">
			parser.WithAttribute(),     // Allows {.class #id} syntax
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(), // Allows raw HTML to pass through
		),
	)
}

func buildDocs(outputDir string, basePath string) error {
	// Get project info
	title := filepath.Base(mustGetCwd())
	description := ""
	repoURL := getGitHubRepoURL()

	// Auto-detect basePath from repo URL if not provided
	// For GitHub Pages project sites: https://user.github.io/repo-name/
	if basePath == "" && repoURL != "" {
		// Extract repo name from URL like https://github.com/user/repo
		parts := strings.Split(strings.TrimSuffix(repoURL, ".git"), "/")
		if len(parts) > 0 {
			repoName := parts[len(parts)-1]
			basePath = "/" + repoName
		}
	}

	// Check for xplat.yaml to get real title/description
	if data, err := os.ReadFile("xplat.yaml"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "name:") {
				title = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			}
			if strings.HasPrefix(line, "description:") {
				description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			}
		}
	}

	md := newMarkdown()

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	// Get list of all docs for sidebar
	docsList := listDocFiles()

	// Build each doc file
	for _, doc := range docsList {
		var mdFile string
		var outFile string

		if doc.Path == "/" {
			// Home page
			mdFile = "README.md"
			outFile = filepath.Join(outputDir, "index.html")
		} else if strings.HasPrefix(doc.Path, "/docs/") {
			// docs/ folder: /docs/FILE → docs/FILE.md → docs/FILE.html
			mdFile = doc.Path[1:] + ".md"
			outFile = filepath.Join(outputDir, doc.Path[1:]+".html")
		} else {
			// Root .md files: /FOO → FOO.md → FOO.html
			mdFile = doc.Path[1:] + ".md"
			outFile = filepath.Join(outputDir, doc.Path[1:]+".html")
		}

		// Read markdown
		content, err := os.ReadFile(mdFile)
		if err != nil {
			fmt.Printf("  ⚠ Skipped %s: %v\n", mdFile, err)
			continue
		}

		// Convert to HTML
		var htmlContent strings.Builder
		if err := md.Convert(content, &htmlContent); err != nil {
			return fmt.Errorf("failed to render %s: %w", mdFile, err)
		}

		// Ensure output directory exists
		if err := os.MkdirAll(filepath.Dir(outFile), 0755); err != nil {
			return fmt.Errorf("failed to create dir for %s: %w", outFile, err)
		}

		// Generate TOC for this page
		tocHTML := generateTOC(content)

		// Write HTML
		html := renderCaymanHTMLWithNav(title, description, htmlContent.String(), doc.Path, docsList, repoURL, tocHTML, basePath)
		if err := os.WriteFile(outFile, []byte(html), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", outFile, err)
		}

		fmt.Printf("  ✓ %s → %s\n", mdFile, outFile)
	}

	// Copy static assets (images, etc)
	if err := copyStaticAssets(outputDir); err != nil {
		fmt.Printf("  ⚠ Static assets: %v\n", err)
	}

	fmt.Printf("\n✅ Built %d pages to %s/\n", len(docsList), outputDir)
	return nil
}

// copyStaticAssets copies images and other static files to output
func copyStaticAssets(outputDir string) error {
	// Copy docs/images if exists
	if _, err := os.Stat("docs/images"); err == nil {
		destDir := filepath.Join(outputDir, "docs", "images")
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return err
		}

		entries, err := os.ReadDir("docs/images")
		if err != nil {
			return err
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			src := filepath.Join("docs/images", e.Name())
			dst := filepath.Join(destDir, e.Name())
			data, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			if err := os.WriteFile(dst, data, 0644); err != nil {
				continue
			}
			fmt.Printf("  ✓ %s → %s\n", src, dst)
		}
	}
	return nil
}

func serveDocs(port string) error {
	// Get project info
	title := filepath.Base(mustGetCwd())
	description := ""
	repoURL := getGitHubRepoURL()

	// Check for xplat.yaml to get real title/description
	if data, err := os.ReadFile("xplat.yaml"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "name:") {
				title = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			}
			if strings.HasPrefix(line, "description:") {
				description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			}
		}
	}

	md := newMarkdown()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Serve static files (images, css, etc)
		if isStaticFile(path) {
			staticPath := "." + path
			if _, err := os.Stat(staticPath); err == nil {
				http.ServeFile(w, r, staticPath)
				return
			}
		}

		// Determine which markdown file to serve
		mdFile := resolveMarkdownFile(path)

		// Read markdown file
		content, err := os.ReadFile(mdFile)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Convert markdown to HTML
		var htmlContent strings.Builder
		if err := md.Convert(content, &htmlContent); err != nil {
			http.Error(w, "Error rendering markdown", 500)
			return
		}

		// Get list of all docs for sidebar
		docsList := listDocFiles()

		// Generate TOC for this page
		tocHTML := generateTOC(content)

		// Render with Cayman-style template
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, renderCaymanHTMLWithNav(title, description, htmlContent.String(), path, docsList, repoURL, tocHTML, ""))
	})

	fmt.Printf("\n📚 Serving docs at http://localhost:%s\n", port)
	fmt.Printf("   Press Ctrl+C to stop\n\n")
	fmt.Printf("   http://localhost:%s/              → README.md\n", port)
	if _, err := os.Stat("docs"); err == nil {
		fmt.Printf("   http://localhost:%s/docs/FILE     → docs/FILE.md\n", port)
	}
	fmt.Println()

	return http.ListenAndServe(":"+port, nil)
}

func mustGetCwd() string {
	wd, _ := os.Getwd()
	return wd
}

func isStaticFile(path string) bool {
	exts := []string{".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".css", ".js", ".woff", ".woff2"}
	for _, ext := range exts {
		if strings.HasSuffix(strings.ToLower(path), ext) {
			return true
		}
	}
	return false
}

func resolveMarkdownFile(path string) string {
	switch {
	case path == "/" || path == "/index.html":
		return "README.md"
	case strings.HasSuffix(path, ".html"):
		// /docs/GENERATION.html -> docs/GENERATION.md
		return strings.TrimSuffix(path[1:], ".html") + ".md"
	case strings.HasPrefix(path, "/docs/") && !strings.Contains(filepath.Base(path), "."):
		// /docs/GENERATION -> docs/GENERATION.md
		return path[1:] + ".md"
	default:
		// Try as-is
		mdFile := strings.TrimPrefix(path, "/")
		if !strings.HasSuffix(mdFile, ".md") {
			mdFile += ".md"
		}
		return mdFile
	}
}

// getGitHubRepoURL extracts the GitHub repo URL from git remote
func getGitHubRepoURL() string {
	// Try to read git remote
	data, err := os.ReadFile(".git/config")
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "url = ") {
			url := strings.TrimPrefix(line, "url = ")
			// Convert git@github.com:user/repo.git to https://github.com/user/repo
			if strings.HasPrefix(url, "git@github.com:") {
				url = strings.TrimPrefix(url, "git@github.com:")
				url = strings.TrimSuffix(url, ".git")
				return "https://github.com/" + url
			}
			// Already https
			if strings.HasPrefix(url, "https://github.com/") {
				return strings.TrimSuffix(url, ".git")
			}
		}
	}
	return ""
}

// generateTOC extracts table of contents from markdown content
func generateTOC(content []byte) string {
	md := goldmark.New(
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)

	doc := md.Parser().Parse(text.NewReader(content))
	tree, err := toc.Inspect(doc, content, toc.MinDepth(2), toc.MaxDepth(3))
	if err != nil || tree == nil || len(tree.Items) == 0 {
		return ""
	}

	// Render TOC as HTML list
	var buf bytes.Buffer
	buf.WriteString(`<ul class="toc">`)
	for _, item := range tree.Items {
		renderTOCItem(&buf, item)
	}
	buf.WriteString(`</ul>`)
	return buf.String()
}

func renderTOCItem(buf *bytes.Buffer, item *toc.Item) {
	buf.WriteString(`<li>`)
	if len(item.ID) > 0 {
		buf.WriteString(fmt.Sprintf(`<a href="#%s">%s</a>`, string(item.ID), string(item.Title)))
	} else {
		buf.WriteString(string(item.Title))
	}
	if len(item.Items) > 0 {
		buf.WriteString(`<ul>`)
		for _, sub := range item.Items {
			renderTOCItem(buf, sub)
		}
		buf.WriteString(`</ul>`)
	}
	buf.WriteString(`</li>`)
}

// docFile represents a documentation file for navigation
type docFile struct {
	Name     string
	Path     string
	Category string // "overview" for root files, "guides" for docs/ folder
	MDFile   string // Source markdown file path
}


// listDocFiles dynamically discovers markdown files in docs/ folder
func listDocFiles() []docFile {
	var docs []docFile

	// Add README as first item (Home)
	if _, err := os.Stat("README.md"); err == nil {
		docs = append(docs, docFile{Name: "Home", Path: "/", Category: "overview", MDFile: "README.md"})
	}

	// Only scan docs/ folder
	entries, err := os.ReadDir("docs")
	if err != nil {
		return docs
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		// Convert SCREAMING_CASE to Title Case
		displayName := strings.ReplaceAll(name, "_", " ")
		docs = append(docs, docFile{
			Name:     displayName,
			Path:     "/docs/" + name,
			Category: "guides",
			MDFile:   "docs/" + e.Name(),
		})
	}

	return docs
}

// findPrevNext finds the previous and next docs relative to current path
func findPrevNext(docs []docFile, currentPath string) (*docFile, *docFile) {
	for i, doc := range docs {
		if doc.Path == currentPath {
			var prev, next *docFile
			if i > 0 {
				prev = &docs[i-1]
			}
			if i < len(docs)-1 {
				next = &docs[i+1]
			}
			return prev, next
		}
	}
	return nil, nil
}

func renderCaymanHTMLWithNav(title, description, content, currentPath string, docs []docFile, repoURL string, tocHTML string, basePath string) string {
	// Find prev/next for navigation
	prev, next := findPrevNext(docs, currentPath)

	// Find current doc for edit link
	var currentMDFile string
	for _, doc := range docs {
		if doc.Path == currentPath {
			currentMDFile = doc.MDFile
			break
		}
	}

	// Build navigation items (with basePath prefix)
	var overviewItems, guideItems []templates.DocsNavItem
	for _, doc := range docs {
		fullPath := basePath + doc.Path
		if doc.Path == "/" && basePath != "" {
			fullPath = basePath + "/"
		}
		item := templates.DocsNavItem{
			Name:   doc.Name,
			Path:   fullPath,
			Active: doc.Path == currentPath || (currentPath == "/" && doc.Path == "/"),
		}
		if doc.Category == "overview" {
			overviewItems = append(overviewItems, item)
		} else {
			guideItems = append(guideItems, item)
		}
	}

	// Build search data JSON (with basePath prefix)
	type searchItem struct {
		Title string `json:"title"`
		Path  string `json:"path"`
	}
	var searchItems []searchItem
	for _, doc := range docs {
		fullPath := basePath + doc.Path
		if doc.Path == "/" && basePath != "" {
			fullPath = basePath + "/"
		}
		searchItems = append(searchItems, searchItem{Title: doc.Name, Path: fullPath})
	}
	searchJSON, _ := json.Marshal(searchItems)

	// Version badge
	versionBadge := ""
	if docsVersion != "" && docsVersion != "dev" {
		versionBadge = docsVersion
	}

	// Edit URL
	editURL := ""
	if repoURL != "" && currentMDFile != "" {
		editURL = repoURL + "/edit/main/" + currentMDFile
	}

	// Prev/Next docs (with basePath prefix)
	var prevDoc, nextDoc *templates.DocsNavItem
	if prev != nil {
		fullPath := basePath + prev.Path
		if prev.Path == "/" && basePath != "" {
			fullPath = basePath + "/"
		}
		prevDoc = &templates.DocsNavItem{Name: prev.Name, Path: fullPath}
	}
	if next != nil {
		fullPath := basePath + next.Path
		if next.Path == "/" && basePath != "" {
			fullPath = basePath + "/"
		}
		nextDoc = &templates.DocsNavItem{Name: next.Name, Path: fullPath}
	}

	// Build template data
	data := templates.DocsPageData{
		Title:          title,
		Description:    description,
		Content:        content,
		RepoURL:        repoURL,
		VersionBadge:   versionBadge,
		EditURL:        editURL,
		TOC:            tocHTML,
		SearchDataJSON: string(searchJSON),
		OverviewItems:  overviewItems,
		GuideItems:     guideItems,
		PrevDoc:        prevDoc,
		NextDoc:        nextDoc,
	}

	// Render template
	result, err := templates.RenderXplat("docs.html.tmpl", data)
	if err != nil {
		// Fallback to simple error page
		return fmt.Sprintf("<html><body><h1>Error</h1><p>%v</p></body></html>", err)
	}
	return string(result)
}

