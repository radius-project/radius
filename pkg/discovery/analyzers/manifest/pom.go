package manifest

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// POM represents a Maven pom.xml file.
type POM struct {
	XMLName              xml.Name     `xml:"project"`
	GroupID              string       `xml:"groupId"`
	ArtifactID           string       `xml:"artifactId"`
	Version              string       `xml:"version"`
	Packaging            string       `xml:"packaging"`
	Parent               *POMParent   `xml:"parent"`
	Dependencies         []Dependency `xml:"dependencies>dependency"`
	DependencyManagement []Dependency `xml:"dependencyManagement>dependencies>dependency"`
	Properties           POMProperties
}

// POMParent represents a parent POM reference.
type POMParent struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

// Dependency represents a Maven dependency.
type Dependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Type       string `xml:"type"`
	Optional   bool   `xml:"optional"`
}

// POMProperties holds project properties.
type POMProperties map[string]string

// UnmarshalXML custom unmarshaler for properties.
func (p *POMProperties) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	*p = make(map[string]string)
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		switch el := t.(type) {
		case xml.StartElement:
			var value string
			if err := d.DecodeElement(&value, &el); err != nil {
				return err
			}
			(*p)[el.Name.Local] = value
		case xml.EndElement:
			if el.Name == start.Name {
				return nil
			}
		}
	}
	return nil
}

// ParsePOM parses a pom.xml file.
func ParsePOM(path string) (*POM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pom.xml: %w", err)
	}

	var pom POM
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil, fmt.Errorf("parsing pom.xml: %w", err)
	}

	return &pom, nil
}

// FindPOM searches for pom.xml files in the project.
func FindPOM(projectPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip common directories
		if info.IsDir() {
			name := info.Name()
			if name == "target" || name == ".git" || name == ".mvn" ||
				name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Name() == "pom.xml" {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// FullName returns groupId:artifactId format.
func (d *Dependency) FullName() string {
	return fmt.Sprintf("%s:%s", d.GroupID, d.ArtifactID)
}

// HasDependency checks if a dependency exists by groupId:artifactId.
func (p *POM) HasDependency(fullName string) bool {
	for _, dep := range p.Dependencies {
		if dep.FullName() == fullName {
			return true
		}
	}
	return false
}

// DependencyVersion returns the version of a dependency.
func (p *POM) DependencyVersion(fullName string) string {
	for _, dep := range p.Dependencies {
		if dep.FullName() == fullName {
			return dep.Version
		}
	}
	return ""
}

// AllDependencies returns all dependencies as a map.
func (p *POM) AllDependencies(includeTest bool) map[string]string {
	result := make(map[string]string)
	for _, dep := range p.Dependencies {
		if !includeTest && strings.ToLower(dep.Scope) == "test" {
			continue
		}
		result[dep.FullName()] = dep.Version
	}
	return result
}

// ResolveVersion resolves property placeholders in versions.
func (p *POM) ResolveVersion(version string) string {
	if !strings.HasPrefix(version, "${") || !strings.HasSuffix(version, "}") {
		return version
	}

	propName := strings.TrimSuffix(strings.TrimPrefix(version, "${"), "}")
	if resolved, ok := p.Properties[propName]; ok {
		return resolved
	}

	return version
}
