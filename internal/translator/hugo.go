// hugo.go - Hugo-specific integration for the translator.
//
// This file provides optional Hugo integration. The core translator
// works with any markdown files and target languages. Hugo integration
// adds automatic language discovery from Hugo config files.
package translator

import (
	"fmt"
	"os"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Using pelletier/go-toml/v2 - same library Hugo uses for TOML v1.0.0 compliance

// HugoConfig represents Hugo-specific configuration paths
type HugoConfig struct {
	LanguagesFile string // e.g., "config/_default/languages.toml"
}

// DefaultHugoConfig returns standard Hugo config paths
func DefaultHugoConfig() *HugoConfig {
	return &HugoConfig{
		LanguagesFile: "config/_default/languages.toml",
	}
}

// HugoLanguageEntry represents a single language in languages.toml
type HugoLanguageEntry struct {
	LanguageName string `toml:"languageName"`
	ContentDir   string `toml:"contentDir"`
	Weight       int    `toml:"weight"`
}

// HugoLanguagesConfig represents the full languages.toml file
// Keys are language codes (en, de, zh, ja)
type HugoLanguagesConfig map[string]HugoLanguageEntry

// ParseHugoLanguages reads language config from Hugo's languages.toml
// Returns target languages, source language code, source content dir, and error.
//
// Hugo languages.toml format:
//
//	[en]
//	languageName = "English"
//	contentDir = "content/english"
//	weight = 1
//
//	[de]
//	languageName = "German"
//	contentDir = "content/german"
//	weight = 2
//
// Weight 1 is treated as the source language; all others are targets.
func ParseHugoLanguages(configPath string) ([]Language, string, string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, "", "", err
	}

	var config HugoLanguagesConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, "", "", err
	}

	var langs []Language
	var sourceLang, sourceDir string

	for code, entry := range config {
		// Extract dir name from "content/english" → "english"
		dirName := strings.TrimPrefix(entry.ContentDir, "content/")

		if entry.Weight == 1 {
			// Weight 1 is source language
			sourceLang = code
			sourceDir = dirName
		} else {
			langs = append(langs, Language{
				Code:    code,
				Name:    entry.LanguageName,
				DirName: dirName,
			})
		}
	}

	return langs, sourceLang, sourceDir, nil
}

// TryLoadHugoConfig attempts to load language config from Hugo.
// Returns nil if Hugo config not found (caller can use defaults).
func TryLoadHugoConfig(config *Config) error {
	hugoConfig := DefaultHugoConfig()

	langs, sourceLang, sourceDir, err := ParseHugoLanguages(hugoConfig.LanguagesFile)
	if err != nil {
		// Not a Hugo project or config not found - that's okay
		return nil
	}

	// Apply Hugo config to translator config
	config.TargetLangs = langs
	if sourceLang != "" {
		config.SourceLang = sourceLang
		config.SourceDir = sourceDir
	}

	return nil
}

// ValidateHugoConfig checks if translator config matches Hugo config.
// Returns list of mismatches or nil if all good.
func ValidateHugoConfig(config *Config) []string {
	hugoConfig := DefaultHugoConfig()

	hugoLangs, hugoSourceLang, hugoSourceDir, err := ParseHugoLanguages(hugoConfig.LanguagesFile)
	if err != nil {
		return []string{fmt.Sprintf("Cannot read Hugo config: %v", err)}
	}

	var mismatches []string

	// Check source language
	if config.SourceLang != hugoSourceLang {
		mismatches = append(mismatches, fmt.Sprintf(
			"Source language mismatch: translator=%s, Hugo=%s",
			config.SourceLang, hugoSourceLang))
	}

	// Check source directory
	if config.SourceDir != hugoSourceDir {
		mismatches = append(mismatches, fmt.Sprintf(
			"Source directory mismatch: translator=%s, Hugo=%s",
			config.SourceDir, hugoSourceDir))
	}

	// Check target languages
	hugoLangMap := make(map[string]Language)
	for _, lang := range hugoLangs {
		hugoLangMap[lang.Code] = lang
	}

	configLangMap := make(map[string]Language)
	for _, lang := range config.TargetLangs {
		configLangMap[lang.Code] = lang
	}

	// Languages in translator but not Hugo
	for code := range configLangMap {
		if _, ok := hugoLangMap[code]; !ok {
			mismatches = append(mismatches, fmt.Sprintf(
				"Language '%s' in translator but not in Hugo config", code))
		}
	}

	// Languages in Hugo but not translator
	for code := range hugoLangMap {
		if _, ok := configLangMap[code]; !ok {
			mismatches = append(mismatches, fmt.Sprintf(
				"Language '%s' in Hugo config but not in translator", code))
		}
	}

	return mismatches
}

// IsHugoProject checks if current directory is a Hugo project
func IsHugoProject() bool {
	hugoConfig := DefaultHugoConfig()
	_, err := os.Stat(hugoConfig.LanguagesFile)
	return err == nil
}

// MenuItem represents a single menu entry in Hugo's menu config
type MenuItem struct {
	Name        string `toml:"name"`
	URL         string `toml:"url"`
	Weight      int    `toml:"weight"`
	Parent      string `toml:"parent"`
	Identifier  string `toml:"identifier"`
	HasChildren bool   `toml:"hasChildren"`
}

// MenuConfig holds parsed menu data for a language
type MenuConfig struct {
	LangCode string
	Main     []MenuItem `toml:"main"`
	Footer   []MenuItem `toml:"footer"`
}

// ParseMenuFile reads and parses a Hugo menu TOML file using proper TOML parsing
func ParseMenuFile(path string) (*MenuConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	config := &MenuConfig{}
	if err := toml.Unmarshal(data, config); err != nil {
		return nil, err
	}
	return config, nil
}

// GetMenuFilePath returns the path to a language's menu config file
func GetMenuFilePath(langCode string) string {
	return fmt.Sprintf("config/_default/menus.%s.toml", langCode)
}

// ValidateMenuLinks checks if menu URLs correspond to existing content files
func ValidateMenuLinks(menu *MenuConfig, langCode string, contentDir string) []string {
	var issues []string

	langDir := "english" // Default for en
	if langCode != "en" {
		// Map lang codes to directory names
		langDirMap := map[string]string{
			"de": "german",
			"zh": "chinese",
			"ja": "japanese",
		}
		if dir, ok := langDirMap[langCode]; ok {
			langDir = dir
		} else {
			langDir = langCode
		}
	}

	checkURL := func(item MenuItem, menuType string) {
		if item.URL == "" {
			return
		}
		// Convert URL to content path
		// /platform → content/{lang}/platform/_index.md or .md
		urlPath := strings.TrimPrefix(item.URL, "/")
		urlPath = strings.TrimSuffix(urlPath, "/")

		// Try _index.md first (section), then .md (page)
		indexPath := fmt.Sprintf("%s/%s/%s/_index.md", contentDir, langDir, urlPath)
		pagePath := fmt.Sprintf("%s/%s/%s.md", contentDir, langDir, urlPath)

		_, indexErr := os.Stat(indexPath)
		_, pageErr := os.Stat(pagePath)

		if indexErr != nil && pageErr != nil {
			issues = append(issues, fmt.Sprintf("[%s] %s: %s → no content file", menuType, item.Name, item.URL))
		}
	}

	for _, item := range menu.Main {
		checkURL(item, "main")
	}
	for _, item := range menu.Footer {
		checkURL(item, "footer")
	}

	return issues
}

// CompareMenuStructure compares English menu with target language menu
func CompareMenuStructure(enMenu, targetMenu *MenuConfig, targetLang string) []string {
	var diffs []string

	// Build maps for comparison
	enMainURLs := make(map[string]MenuItem)
	targetMainURLs := make(map[string]MenuItem)

	for _, item := range enMenu.Main {
		if item.URL != "" {
			enMainURLs[item.URL] = item
		}
	}
	for _, item := range targetMenu.Main {
		if item.URL != "" {
			targetMainURLs[item.URL] = item
		}
	}

	// Check for URLs in English but not in target
	for url := range enMainURLs {
		if _, ok := targetMainURLs[url]; !ok {
			diffs = append(diffs, fmt.Sprintf("Missing in %s: %s", targetLang, url))
		}
	}

	// Check for URLs in target but not in English
	for url := range targetMainURLs {
		if _, ok := enMainURLs[url]; !ok {
			diffs = append(diffs, fmt.Sprintf("Extra in %s (not in EN): %s", targetLang, url))
		}
	}

	return diffs
}

// MenuTranslations maps English menu item names to translations
var MenuTranslations = map[string]map[string]string{
	"ja": {
		"Platform": "プラットフォーム", "Applications": "アプリケーション", "Technology": "テクノロジー",
		"Company": "会社", "Blog": "ブログ", "Contact": "お問い合わせ",
		"Publish": "パブリッシュ", "Spatial": "スペーシャル", "Foundation": "ファウンデーション",
		"Sensing": "センシング", "Robotics": "ロボティクス", "Simulation": "シミュレーション",
		"Digital Twins": "デジタルツイン", "Manufacturing": "製造業", "Construction": "建設",
		"Government": "政府", "Healthcare": "ヘルスケア", "Financial": "金融",
		"Education": "教育", "Insurance": "保険", "Overview": "概要",
		"Experience": "経験", "Founder": "創業者", "Advisors": "アドバイザー",
		"Get Started": "はじめる", "Privacy Policy": "プライバシーポリシー", "Security": "セキュリティ",
		"Linux & Cross-Platform": "Linux＆クロスプラットフォーム",
	},
	"de": {
		"Platform": "Plattform", "Applications": "Anwendungen", "Technology": "Technologie",
		"Company": "Unternehmen", "Blog": "Blog", "Contact": "Kontakt",
		"Publish": "Veröffentlichen", "Spatial": "Spatial", "Foundation": "Foundation",
		"Sensing": "Sensorik", "Robotics": "Robotik", "Simulation": "Simulation",
		"Digital Twins": "Digitale Zwillinge", "Manufacturing": "Fertigung", "Construction": "Bauwesen",
		"Government": "Regierung", "Healthcare": "Gesundheitswesen", "Financial": "Finanzwesen",
		"Education": "Bildung", "Insurance": "Versicherung", "Overview": "Übersicht",
		"Experience": "Erfahrung", "Founder": "Gründer", "Advisors": "Berater",
		"Get Started": "Loslegen", "Privacy Policy": "Datenschutz", "Security": "Sicherheit",
		"Linux & Cross-Platform": "Linux & Plattformübergreifend",
	},
	"zh": {
		"Platform": "平台", "Applications": "应用", "Technology": "技术",
		"Company": "公司", "Blog": "博客", "Contact": "联系我们",
		"Publish": "发布", "Spatial": "空间", "Foundation": "基础",
		"Sensing": "传感", "Robotics": "机器人", "Simulation": "仿真",
		"Digital Twins": "数字孪生", "Manufacturing": "制造业", "Construction": "建筑",
		"Government": "政府", "Healthcare": "医疗", "Financial": "金融",
		"Education": "教育", "Insurance": "保险", "Overview": "概览",
		"Experience": "经验", "Founder": "创始人", "Advisors": "顾问",
		"Get Started": "开始使用", "Privacy Policy": "隐私政策", "Security": "安全",
		"Linux & Cross-Platform": "Linux与跨平台",
	},
}

// TranslateMenuName translates an English menu item name to target language
func TranslateMenuName(name, langCode string) string {
	if translations, ok := MenuTranslations[langCode]; ok {
		if translated, ok := translations[name]; ok {
			return translated
		}
	}
	return name // Fallback to English if no translation
}

// GenerateMenuFile creates menu TOML content from English menu for target language
func GenerateMenuFile(enMenu *MenuConfig, langCode string) string {
	var sb strings.Builder

	langNameMap := map[string]string{
		"ja": "Japanese",
		"de": "German",
		"zh": "Chinese",
	}
	langName := langNameMap[langCode]
	if langName == "" {
		langName = langCode
	}

	sb.WriteString(fmt.Sprintf("############# %s navigation ##############\n\n", langName))
	sb.WriteString("# main menu\n")
	sb.WriteString("# Note: \"Home\" removed - logo already links to home, saves nav space\n\n")

	// Generate main menu items
	for _, item := range enMenu.Main {
		sb.WriteString("[[main]]\n")
		sb.WriteString(fmt.Sprintf("name = \"%s\"\n", TranslateMenuName(item.Name, langCode)))
		if item.Identifier != "" {
			sb.WriteString(fmt.Sprintf("identifier = \"%s\"\n", item.Identifier))
		}
		if item.URL != "" {
			sb.WriteString(fmt.Sprintf("url = \"%s\"\n", item.URL))
		}
		if item.Parent != "" {
			sb.WriteString(fmt.Sprintf("parent = \"%s\"\n", TranslateMenuName(item.Parent, langCode)))
		}
		sb.WriteString(fmt.Sprintf("weight = %d\n", item.Weight))
		if item.HasChildren {
			sb.WriteString("hasChildren = true\n")
		}
		sb.WriteString("\n")
	}

	// Generate footer menu items
	sb.WriteString("\n# footer menu\n")
	for _, item := range enMenu.Footer {
		sb.WriteString("[[footer]]\n")
		sb.WriteString(fmt.Sprintf("name = \"%s\"\n", TranslateMenuName(item.Name, langCode)))
		sb.WriteString(fmt.Sprintf("url = \"%s\"\n", item.URL))
		sb.WriteString(fmt.Sprintf("weight = %d\n", item.Weight))
		sb.WriteString("\n")
	}

	return sb.String()
}

// ============================================================================
// Language Management Functions
// ============================================================================

// GetLanguageByCode returns the language entry for a given code from languages.toml
// Returns nil if not found
func GetLanguageByCode(code string) (*HugoLanguageEntry, error) {
	hugoConfig := DefaultHugoConfig()
	data, err := os.ReadFile(hugoConfig.LanguagesFile)
	if err != nil {
		return nil, err
	}

	var config HugoLanguagesConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if entry, ok := config[code]; ok {
		return &entry, nil
	}
	return nil, nil
}

// GetNextWeight returns the next available weight for a new language
func GetNextWeight() (int, error) {
	hugoConfig := DefaultHugoConfig()
	data, err := os.ReadFile(hugoConfig.LanguagesFile)
	if err != nil {
		return 2, nil // Default to 2 if no file
	}

	var config HugoLanguagesConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return 2, nil
	}

	maxWeight := 1
	for _, entry := range config {
		if entry.Weight > maxWeight {
			maxWeight = entry.Weight
		}
	}
	return maxWeight + 1, nil
}

// AddLanguageToHugo adds a new language entry to languages.toml
func AddLanguageToHugo(code, name, dirname string) error {
	hugoConfig := DefaultHugoConfig()

	// Read existing file
	data, err := os.ReadFile(hugoConfig.LanguagesFile)
	if err != nil {
		return err
	}

	// Check if language already exists
	var config HugoLanguagesConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return err
	}
	if _, exists := config[code]; exists {
		return fmt.Errorf("language '%s' already exists", code)
	}

	// Get next weight
	weight, _ := GetNextWeight()

	// Append new language to end of file
	var sb strings.Builder
	sb.Write(data)
	sb.WriteString(fmt.Sprintf("\n################ %s ##################\n", name))
	sb.WriteString(fmt.Sprintf("[%s]\n", code))
	sb.WriteString(fmt.Sprintf("languageName = \"%s\"\n", name))
	sb.WriteString(fmt.Sprintf("languageCode = \"%s\"\n", code))
	sb.WriteString(fmt.Sprintf("contentDir = \"content/%s\"\n", dirname))
	sb.WriteString(fmt.Sprintf("weight = %d\n", weight))

	// Check if CJK language
	if code == "zh" || code == "ja" || code == "ko" {
		sb.WriteString("hasCJKLanguage = true\n")
	}

	return os.WriteFile(hugoConfig.LanguagesFile, []byte(sb.String()), 0644)
}

// RemoveLanguageFromHugo removes a language entry from languages.toml
func RemoveLanguageFromHugo(code string) error {
	hugoConfig := DefaultHugoConfig()

	// Read existing file
	data, err := os.ReadFile(hugoConfig.LanguagesFile)
	if err != nil {
		return err
	}

	// Check if language exists
	var config HugoLanguagesConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		return err
	}
	if _, exists := config[code]; !exists {
		return fmt.Errorf("language '%s' not found", code)
	}

	// Can't remove source language
	entry := config[code]
	if entry.Weight == 1 {
		return fmt.Errorf("cannot remove source language '%s'", code)
	}

	// Reconstruct file without the removed language
	var sb strings.Builder
	for langCode, langEntry := range config {
		if langCode == code {
			continue // Skip the one we're removing
		}
		sb.WriteString(fmt.Sprintf("\n################ %s ##################\n", langEntry.LanguageName))
		sb.WriteString(fmt.Sprintf("[%s]\n", langCode))
		sb.WriteString(fmt.Sprintf("languageName = \"%s\"\n", langEntry.LanguageName))
		sb.WriteString(fmt.Sprintf("languageCode = \"%s\"\n", langCode))
		sb.WriteString(fmt.Sprintf("contentDir = \"%s\"\n", langEntry.ContentDir))
		sb.WriteString(fmt.Sprintf("weight = %d\n", langEntry.Weight))

		// Check if CJK language
		if langCode == "zh" || langCode == "ja" || langCode == "ko" {
			sb.WriteString("hasCJKLanguage = true\n")
		}
	}

	return os.WriteFile(hugoConfig.LanguagesFile, []byte(sb.String()), 0644)
}

// CreateContentDirectory creates the content directory for a language with initial _index.md
func CreateContentDirectory(dirname, langName string) error {
	dir := fmt.Sprintf("content/%s", dirname)

	// Create directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Create _index.md with basic front matter
	indexPath := fmt.Sprintf("%s/_index.md", dir)
	content := fmt.Sprintf(`---
title: "%s"
---
`, langName)

	return os.WriteFile(indexPath, []byte(content), 0644)
}
