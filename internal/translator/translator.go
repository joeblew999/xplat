package translator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Language represents a target language for translation
type Language struct {
	Code    string // ISO code: de, zh, ja, etc.
	Name    string // Full name: German, Chinese, etc.
	DirName string // Content directory name: german, chinese, etc.
}

// Config holds translation configuration
type Config struct {
	SourceLang    string
	SourceDir     string // english
	TargetLangs   []Language
	ContentDir    string
	I18nDir       string
	CheckpointTag string
}

// DefaultConfig returns the default configuration.
// If in a Hugo project, reads languages from config/_default/languages.toml.
// Otherwise uses sensible defaults for standalone markdown translation.
func DefaultConfig() *Config {
	config := &Config{
		SourceLang:    "en",
		SourceDir:     "english",
		ContentDir:    "content",
		I18nDir:       "i18n",
		CheckpointTag: "last-translation",
		// Default fallback languages (used when not a Hugo project)
		TargetLangs: []Language{
			{Code: "de", Name: "German", DirName: "german"},
			{Code: "zh", Name: "Chinese", DirName: "chinese"},
			{Code: "ja", Name: "Japanese", DirName: "japanese"},
		},
	}

	// Try to load from Hugo config (overwrites defaults if found)
	TryLoadHugoConfig(config)

	return config
}

// GetLanguageName returns the full name for a language code
func (c *Config) GetLanguageName(code string) string {
	for _, lang := range c.TargetLangs {
		if lang.Code == code {
			return lang.Name
		}
	}
	return code
}

// GetLanguageDir returns the content directory name for a language code
func (c *Config) GetLanguageDir(code string) string {
	for _, lang := range c.TargetLangs {
		if lang.Code == code {
			return lang.DirName
		}
	}
	return code
}

// GetTargetPath converts a source path to a target language path
// content/english/blog/post.md → content/german/blog/post.md
func (c *Config) GetTargetPath(sourcePath, targetLangCode string) string {
	targetDir := c.GetLanguageDir(targetLangCode)
	relPath := strings.TrimPrefix(sourcePath, filepath.Join(c.ContentDir, c.SourceDir)+string(os.PathSeparator))
	return filepath.Join(c.ContentDir, targetDir, relPath)
}

// Translator handles translation operations (requires Claude API key)
type Translator struct {
	apiKey string
	config *Config
	claude *ClaudeClient
	git    *GitManager
}

// New creates a new Translator instance for automated translation
func New(apiKey string) (*Translator, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	config := DefaultConfig()

	// Create Claude client
	claude, err := NewClaudeClient(apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create Claude client: %w", err)
	}

	// Create Git manager
	git, err := NewGitManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create Git manager: %w", err)
	}

	return &Translator{
		apiKey: apiKey,
		config: config,
		claude: claude,
		git:    git,
	}, nil
}

// Check shows which English files have changed since last translation
func (t *Translator) Check() error {
	fmt.Println("Checking for changes since last translation...")

	sourcePath := filepath.Join(t.config.ContentDir, t.config.SourceDir)
	changes, err := t.git.GetChangedFiles(t.config.CheckpointTag, sourcePath)
	if err != nil {
		return fmt.Errorf("failed to get changed files: %w", err)
	}

	if len(changes) == 0 {
		fmt.Println("No changes detected. All content is up to date!")
		return nil
	}

	fmt.Printf("\nFound %d changed files:\n\n", len(changes))
	for _, file := range changes {
		fmt.Printf("  - %s\n", file)
	}
	fmt.Println()

	return nil
}

// TranslateAll translates all changed English content to all target languages
func (t *Translator) TranslateAll() error {
	sourcePath := filepath.Join(t.config.ContentDir, t.config.SourceDir)
	changes, err := t.git.GetChangedFiles(t.config.CheckpointTag, sourcePath)
	if err != nil {
		return fmt.Errorf("failed to get changed files: %w", err)
	}

	if len(changes) == 0 {
		fmt.Println("No changes detected. Nothing to translate.")
		return nil
	}

	fmt.Printf("Translating %d files to %d languages...\n\n", len(changes), len(t.config.TargetLangs))

	for _, lang := range t.config.TargetLangs {
		if err := t.translateFiles(changes, lang); err != nil {
			return fmt.Errorf("failed to translate to %s: %w", lang.Name, err)
		}
	}

	// Update checkpoint
	if err := t.git.UpdateCheckpoint(t.config.CheckpointTag, changes); err != nil {
		return fmt.Errorf("failed to update checkpoint: %w", err)
	}

	return nil
}

// TranslateLang translates changed English content to a specific language
func (t *Translator) TranslateLang(targetLangCode string) error {
	// Find the target language
	var targetLang *Language
	for _, lang := range t.config.TargetLangs {
		if lang.Code == targetLangCode {
			targetLang = &lang
			break
		}
	}
	if targetLang == nil {
		codes := make([]string, len(t.config.TargetLangs))
		for i, lang := range t.config.TargetLangs {
			codes[i] = lang.Code
		}
		return fmt.Errorf("invalid target language: %s (valid: %v)", targetLangCode, codes)
	}

	sourcePath := filepath.Join(t.config.ContentDir, t.config.SourceDir)
	changes, err := t.git.GetChangedFiles(t.config.CheckpointTag, sourcePath)
	if err != nil {
		return fmt.Errorf("failed to get changed files: %w", err)
	}

	if len(changes) == 0 {
		fmt.Println("No changes detected. Nothing to translate.")
		return nil
	}

	return t.translateFiles(changes, *targetLang)
}

// translateFiles translates a list of files to a target language
func (t *Translator) translateFiles(files []string, lang Language) error {
	fmt.Printf("Translating to %s (%s)...\n", lang.Name, lang.Code)

	for i, file := range files {
		fmt.Printf("  [%d/%d] %s\n", i+1, len(files), filepath.Base(file))

		// Read source file
		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}

		// Parse markdown with front matter
		md, err := ParseMarkdown(content)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", file, err)
		}

		// Translate the content (not front matter or code blocks)
		translatedBody, err := t.claude.Translate(md.Body, lang.Code, lang.Name)
		if err != nil {
			return fmt.Errorf("failed to translate %s: %w", file, err)
		}

		// Reconstruct markdown with translated content
		md.Body = translatedBody
		output, err := md.Reconstruct()
		if err != nil {
			return fmt.Errorf("failed to reconstruct %s: %w", file, err)
		}

		// Write to target language directory (preserving path structure)
		targetFile := t.config.GetTargetPath(file, lang.Code)
		targetFileDir := filepath.Dir(targetFile)

		if err := os.MkdirAll(targetFileDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", targetFileDir, err)
		}

		if err := os.WriteFile(targetFile, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", targetFile, err)
		}
	}

	return nil
}

// TranslateI18n translates i18n YAML files
func (t *Translator) TranslateI18n() error {
	sourceFile := filepath.Join(t.config.I18nDir, "en.yaml")

	// Check if source file exists
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		return fmt.Errorf("source i18n file not found: %s", sourceFile)
	}

	for _, lang := range t.config.TargetLangs {
		fmt.Printf("Translating i18n to %s (%s)...\n", lang.Name, lang.Code)

		// Read source i18n file
		content, err := os.ReadFile(sourceFile)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", sourceFile, err)
		}

		// Parse i18n file
		i18nData, err := ParseI18n(content)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", sourceFile, err)
		}

		// Translate values
		translatedData, err := t.claude.TranslateI18n(i18nData, lang.Code, lang.Name)
		if err != nil {
			return fmt.Errorf("failed to translate i18n to %s: %w", lang.Name, err)
		}

		// Write translated i18n file
		targetFile := filepath.Join(t.config.I18nDir, fmt.Sprintf("%s.yaml", lang.Code))
		output, err := ReconstructI18n(translatedData)
		if err != nil {
			return fmt.Errorf("failed to reconstruct i18n: %w", err)
		}

		if err := os.WriteFile(targetFile, []byte(output), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", targetFile, err)
		}
	}

	return nil
}
