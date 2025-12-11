package translator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMenuFile(t *testing.T) {
	// Create a temp menu file
	content := `############# Test navigation ##############

# main menu
[[main]]
name = "Platform"
url = "/platform"
weight = 2
hasChildren = true

[[main]]
name = "Publish"
url = "/platform/publish"
parent = "Platform"
weight = 1

[[main]]
name = "Overview"
identifier = "tech-overview"
url = "/technology"
parent = "Technology"
weight = 1

# footer menu
[[footer]]
name = "Contact"
url = "/contact"
weight = 1
`
	tmpDir := t.TempDir()
	menuFile := filepath.Join(tmpDir, "menus.test.toml")
	if err := os.WriteFile(menuFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test menu file: %v", err)
	}

	// Parse the file
	menu, err := ParseMenuFile(menuFile)
	if err != nil {
		t.Fatalf("ParseMenuFile failed: %v", err)
	}

	// Verify main menu items
	if len(menu.Main) != 3 {
		t.Errorf("Expected 3 main menu items, got %d", len(menu.Main))
	}

	// Check first item
	if menu.Main[0].Name != "Platform" {
		t.Errorf("Expected first item name 'Platform', got '%s'", menu.Main[0].Name)
	}
	if menu.Main[0].URL != "/platform" {
		t.Errorf("Expected first item URL '/platform', got '%s'", menu.Main[0].URL)
	}
	if menu.Main[0].Weight != 2 {
		t.Errorf("Expected first item weight 2, got %d", menu.Main[0].Weight)
	}
	if !menu.Main[0].HasChildren {
		t.Error("Expected first item hasChildren to be true")
	}

	// Check item with parent
	if menu.Main[1].Parent != "Platform" {
		t.Errorf("Expected second item parent 'Platform', got '%s'", menu.Main[1].Parent)
	}

	// Check item with identifier
	if menu.Main[2].Identifier != "tech-overview" {
		t.Errorf("Expected third item identifier 'tech-overview', got '%s'", menu.Main[2].Identifier)
	}

	// Verify footer menu items
	if len(menu.Footer) != 1 {
		t.Errorf("Expected 1 footer menu item, got %d", len(menu.Footer))
	}
	if menu.Footer[0].Name != "Contact" {
		t.Errorf("Expected footer item name 'Contact', got '%s'", menu.Footer[0].Name)
	}
}

func TestParseMenuFile_NotFound(t *testing.T) {
	_, err := ParseMenuFile("/nonexistent/path/menus.toml")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestCompareMenuStructure(t *testing.T) {
	enMenu := &MenuConfig{
		Main: []MenuItem{
			{Name: "Platform", URL: "/platform"},
			{Name: "Technology", URL: "/technology"},
			{Name: "Blog", URL: "/blog"},
		},
	}

	// Target missing /technology
	targetMissing := &MenuConfig{
		Main: []MenuItem{
			{Name: "Plattform", URL: "/platform"},
			{Name: "Blog", URL: "/blog"},
		},
	}

	diffs := CompareMenuStructure(enMenu, targetMissing, "de")
	if len(diffs) != 1 {
		t.Errorf("Expected 1 difference, got %d", len(diffs))
	}
	if len(diffs) > 0 && !strings.Contains(diffs[0], "/technology") {
		t.Errorf("Expected diff about /technology, got '%s'", diffs[0])
	}

	// Target has extra item
	targetExtra := &MenuConfig{
		Main: []MenuItem{
			{Name: "Plattform", URL: "/platform"},
			{Name: "Technologie", URL: "/technology"},
			{Name: "Blog", URL: "/blog"},
			{Name: "Extra", URL: "/extra"},
		},
	}

	diffs = CompareMenuStructure(enMenu, targetExtra, "de")
	if len(diffs) != 1 {
		t.Errorf("Expected 1 difference, got %d", len(diffs))
	}
	if len(diffs) > 0 && !strings.Contains(diffs[0], "/extra") {
		t.Errorf("Expected diff about /extra, got '%s'", diffs[0])
	}

	// Target in sync
	targetSync := &MenuConfig{
		Main: []MenuItem{
			{Name: "Plattform", URL: "/platform"},
			{Name: "Technologie", URL: "/technology"},
			{Name: "Blog", URL: "/blog"},
		},
	}

	diffs = CompareMenuStructure(enMenu, targetSync, "de")
	if len(diffs) != 0 {
		t.Errorf("Expected 0 differences for synced menus, got %d: %v", len(diffs), diffs)
	}
}

func TestTranslateMenuName(t *testing.T) {
	tests := []struct {
		name     string
		langCode string
		expected string
	}{
		{"Platform", "ja", "プラットフォーム"},
		{"Platform", "de", "Plattform"},
		{"Platform", "zh", "平台"},
		{"Blog", "ja", "ブログ"},
		{"Blog", "de", "Blog"}, // Same in German
		{"Unknown", "ja", "Unknown"}, // Fallback to English
		{"Platform", "xx", "Platform"}, // Unknown language
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.langCode, func(t *testing.T) {
			result := TranslateMenuName(tt.name, tt.langCode)
			if result != tt.expected {
				t.Errorf("TranslateMenuName(%q, %q) = %q, want %q",
					tt.name, tt.langCode, result, tt.expected)
			}
		})
	}
}

func TestGenerateMenuFile(t *testing.T) {
	enMenu := &MenuConfig{
		Main: []MenuItem{
			{Name: "Platform", URL: "/platform", Weight: 1, HasChildren: true},
			{Name: "Publish", URL: "/platform/publish", Parent: "Platform", Weight: 1},
		},
		Footer: []MenuItem{
			{Name: "Contact", URL: "/contact", Weight: 1},
		},
	}

	// Generate Japanese menu
	content := GenerateMenuFile(enMenu, "ja")

	// Check header
	if !strings.Contains(content, "Japanese navigation") {
		t.Error("Expected Japanese header in generated content")
	}

	// Check translations
	if !strings.Contains(content, `name = "プラットフォーム"`) {
		t.Error("Expected translated Platform name")
	}
	if !strings.Contains(content, `name = "パブリッシュ"`) {
		t.Error("Expected translated Publish name")
	}
	if !strings.Contains(content, `parent = "プラットフォーム"`) {
		t.Error("Expected translated parent name")
	}
	if !strings.Contains(content, `name = "お問い合わせ"`) {
		t.Error("Expected translated Contact name")
	}

	// Check structure preserved
	if !strings.Contains(content, `url = "/platform"`) {
		t.Error("Expected URL preserved")
	}
	if !strings.Contains(content, "hasChildren = true") {
		t.Error("Expected hasChildren preserved")
	}
}

func TestValidateMenuLinks(t *testing.T) {
	// Create temp content directory with some files
	tmpDir := t.TempDir()
	contentDir := filepath.Join(tmpDir, "content")
	englishDir := filepath.Join(contentDir, "english")

	// Create test content files
	if err := os.MkdirAll(filepath.Join(englishDir, "platform"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(englishDir, "platform", "_index.md"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(englishDir, "blog"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(englishDir, "blog", "_index.md"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	menu := &MenuConfig{
		Main: []MenuItem{
			{Name: "Platform", URL: "/platform"},         // exists
			{Name: "Blog", URL: "/blog"},                 // exists
			{Name: "Missing", URL: "/missing"},           // doesn't exist
			{Name: "No URL", URL: ""},                    // no URL, should skip
		},
		Footer: []MenuItem{
			{Name: "Contact", URL: "/contact"},           // doesn't exist
		},
	}

	issues := ValidateMenuLinks(menu, "en", contentDir)

	// Should find 2 issues: /missing and /contact
	if len(issues) != 2 {
		t.Errorf("Expected 2 issues, got %d: %v", len(issues), issues)
	}

	// Check that the issues mention the right URLs
	foundMissing := false
	foundContact := false
	for _, issue := range issues {
		if strings.Contains(issue, "/missing") {
			foundMissing = true
		}
		if strings.Contains(issue, "/contact") {
			foundContact = true
		}
	}
	if !foundMissing {
		t.Error("Expected issue about /missing")
	}
	if !foundContact {
		t.Error("Expected issue about /contact")
	}
}

func TestGetMenuFilePath(t *testing.T) {
	tests := []struct {
		langCode string
		expected string
	}{
		{"en", "config/_default/menus.en.toml"},
		{"ja", "config/_default/menus.ja.toml"},
		{"de", "config/_default/menus.de.toml"},
	}

	for _, tt := range tests {
		result := GetMenuFilePath(tt.langCode)
		if result != tt.expected {
			t.Errorf("GetMenuFilePath(%q) = %q, want %q", tt.langCode, result, tt.expected)
		}
	}
}

func TestParseHugoLanguages(t *testing.T) {
	// Create a temp languages.toml file
	content := `[en]
languageName = "English"
contentDir = "content/english"
weight = 1

[de]
languageName = "German"
contentDir = "content/german"
weight = 2

[ja]
languageName = "Japanese"
contentDir = "content/japanese"
weight = 3
`
	tmpDir := t.TempDir()
	langFile := filepath.Join(tmpDir, "languages.toml")
	if err := os.WriteFile(langFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test languages file: %v", err)
	}

	// Parse the file
	langs, sourceLang, sourceDir, err := ParseHugoLanguages(langFile)
	if err != nil {
		t.Fatalf("ParseHugoLanguages failed: %v", err)
	}

	// Verify source language (weight 1)
	if sourceLang != "en" {
		t.Errorf("Expected source language 'en', got '%s'", sourceLang)
	}
	if sourceDir != "english" {
		t.Errorf("Expected source dir 'english', got '%s'", sourceDir)
	}

	// Verify target languages (weight > 1)
	if len(langs) != 2 {
		t.Errorf("Expected 2 target languages, got %d", len(langs))
	}

	// Check that de and ja are in targets (order may vary due to map iteration)
	langCodes := make(map[string]bool)
	for _, lang := range langs {
		langCodes[lang.Code] = true
		// Verify dir name extracted correctly
		if lang.Code == "de" && lang.DirName != "german" {
			t.Errorf("Expected German dir 'german', got '%s'", lang.DirName)
		}
		if lang.Code == "ja" && lang.DirName != "japanese" {
			t.Errorf("Expected Japanese dir 'japanese', got '%s'", lang.DirName)
		}
	}
	if !langCodes["de"] {
		t.Error("Expected 'de' in target languages")
	}
	if !langCodes["ja"] {
		t.Error("Expected 'ja' in target languages")
	}
}

func TestParseHugoLanguages_NotFound(t *testing.T) {
	_, _, _, err := ParseHugoLanguages("/nonexistent/path/languages.toml")
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}
}

func TestParseHugoLanguages_InvalidTOML(t *testing.T) {
	// Create an invalid TOML file
	content := `[en
languageName = "English"
`
	tmpDir := t.TempDir()
	langFile := filepath.Join(tmpDir, "languages.toml")
	if err := os.WriteFile(langFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, _, _, err := ParseHugoLanguages(langFile)
	if err == nil {
		t.Error("Expected error for invalid TOML, got nil")
	}
}

// Test parsing real menu file from project
func TestParseRealMenuFile(t *testing.T) {
	// Skip if not in project directory
	menuPath := "config/_default/menus.en.toml"
	if _, err := os.Stat(menuPath); os.IsNotExist(err) {
		t.Skip("Not in project directory, skipping real file test")
	}

	menu, err := ParseMenuFile(menuPath)
	if err != nil {
		t.Fatalf("Failed to parse real menu file: %v", err)
	}

	// Sanity checks on real file
	if len(menu.Main) == 0 {
		t.Error("Expected main menu items in real file")
	}
	if len(menu.Footer) == 0 {
		t.Error("Expected footer menu items in real file")
	}

	// Check known items exist
	foundPlatform := false
	for _, item := range menu.Main {
		if item.Name == "Platform" {
			foundPlatform = true
			if item.URL != "/platform" {
				t.Errorf("Platform URL = %q, want /platform", item.URL)
			}
		}
	}
	if !foundPlatform {
		t.Error("Expected to find Platform in main menu")
	}
}
