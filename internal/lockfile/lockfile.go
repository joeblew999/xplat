// Package lockfile manages xplat-lock.yaml for tracking installed packages.
package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

const FileName = "xplat-lock.yaml"

// Lockfile tracks installed packages and their configuration.
type Lockfile struct {
	Version  string             `yaml:"version"`
	Updated  time.Time          `yaml:"updated"`
	Packages map[string]Package `yaml:"packages"`
}

// Package represents an installed package.
type Package struct {
	Name        string   `yaml:"name"`
	Version     string   `yaml:"version"`
	Source      string   `yaml:"source"`                 // e.g., "github:joeblew999/plat-nats"
	InstalledAt time.Time `yaml:"installed_at"`
	Binary      *Binary  `yaml:"binary,omitempty"`
	Taskfile    *Taskfile `yaml:"taskfile,omitempty"`
	Process     *Process `yaml:"process,omitempty"`
}

// Binary represents an installed binary.
type Binary struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// Taskfile represents taskfile configuration.
type Taskfile struct {
	Path      string `yaml:"path"`
	Namespace string `yaml:"namespace,omitempty"`
	URL       string `yaml:"url"` // Remote URL for includes
}

// Process represents process configuration.
type Process struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

// Load reads the lockfile from the given directory.
func Load(dir string) (*Lockfile, error) {
	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Lockfile{
				Version:  "1",
				Packages: make(map[string]Package),
			}, nil
		}
		return nil, fmt.Errorf("failed to read lockfile: %w", err)
	}

	var lf Lockfile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("failed to parse lockfile: %w", err)
	}

	if lf.Packages == nil {
		lf.Packages = make(map[string]Package)
	}

	return &lf, nil
}

// Save writes the lockfile to the given directory.
func (lf *Lockfile) Save(dir string) error {
	lf.Updated = time.Now()

	data, err := yaml.Marshal(lf)
	if err != nil {
		return fmt.Errorf("failed to marshal lockfile: %w", err)
	}

	header := "# xplat-lock.yaml - Tracks installed packages\n# Do not edit manually. Regenerate with: xplat pkg install\n\n"
	path := filepath.Join(dir, FileName)

	if err := os.WriteFile(path, append([]byte(header), data...), 0644); err != nil {
		return fmt.Errorf("failed to write lockfile: %w", err)
	}

	return nil
}

// AddPackage adds or updates a package in the lockfile.
func (lf *Lockfile) AddPackage(pkg Package) {
	pkg.InstalledAt = time.Now()
	lf.Packages[pkg.Name] = pkg
}

// RemovePackage removes a package from the lockfile.
func (lf *Lockfile) RemovePackage(name string) {
	delete(lf.Packages, name)
}

// HasPackage returns true if the package is installed.
func (lf *Lockfile) HasPackage(name string) bool {
	_, ok := lf.Packages[name]
	return ok
}

// GetPackage returns a package by name.
func (lf *Lockfile) GetPackage(name string) (Package, bool) {
	pkg, ok := lf.Packages[name]
	return pkg, ok
}

// ListPackages returns all installed packages sorted by name.
func (lf *Lockfile) ListPackages() []Package {
	var pkgs []Package
	for _, pkg := range lf.Packages {
		pkgs = append(pkgs, pkg)
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Name < pkgs[j].Name
	})
	return pkgs
}

// PackagesWithTaskfile returns packages that have taskfile configuration.
func (lf *Lockfile) PackagesWithTaskfile() []Package {
	var pkgs []Package
	for _, pkg := range lf.Packages {
		if pkg.Taskfile != nil {
			pkgs = append(pkgs, pkg)
		}
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Name < pkgs[j].Name
	})
	return pkgs
}

// PackagesWithProcess returns packages that have process configuration.
func (lf *Lockfile) PackagesWithProcess() []Package {
	var pkgs []Package
	for _, pkg := range lf.Packages {
		if pkg.Process != nil {
			pkgs = append(pkgs, pkg)
		}
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].Name < pkgs[j].Name
	})
	return pkgs
}
