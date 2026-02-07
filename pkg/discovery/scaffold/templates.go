// Package scaffold provides application scaffolding functionality.
package scaffold

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/radius-project/radius/pkg/discovery/practices"
)

// ScaffoldOptions configures the scaffolding behavior.
type ScaffoldOptions struct {
	// Name is the application name.
	Name string

	// OutputPath is where to create the scaffold.
	OutputPath string

	// Environment is the target environment.
	Environment string

	// Template is the application template to use.
	Template string

	// Dependencies is the list of infrastructure dependencies.
	Dependencies []string
}

// ScaffoldResult contains the result of scaffolding.
type ScaffoldResult struct {
	// CreatedFiles lists all files created.
	CreatedFiles []string

	// AppBicepPath is the path to the generated app.bicep.
	AppBicepPath string

	// RadiusDir is the path to the .radius directory.
	RadiusDir string
}

// Scaffolder creates new application scaffolds.
type Scaffolder struct {
	opts      ScaffoldOptions
	practices *practices.TeamPractices
}

// NewScaffolder creates a new Scaffolder.
func NewScaffolder(opts ScaffoldOptions) *Scaffolder {
	return &Scaffolder{opts: opts}
}

// WithPractices sets team practices to apply.
func (s *Scaffolder) WithPractices(p *practices.TeamPractices) *Scaffolder {
	s.practices = p
	return s
}

// Scaffold creates the application scaffold.
func (s *Scaffolder) Scaffold(ctx context.Context) (*ScaffoldResult, error) {
	result := &ScaffoldResult{}

	// Create output directory
	if err := os.MkdirAll(s.opts.OutputPath, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Create .radius directory
	radiusDir := filepath.Join(s.opts.OutputPath, ".radius")
	if err := os.MkdirAll(radiusDir, 0755); err != nil {
		return nil, fmt.Errorf("creating .radius directory: %w", err)
	}
	result.RadiusDir = radiusDir

	// Generate app.bicep
	appBicepPath := filepath.Join(s.opts.OutputPath, "app.bicep")
	if err := s.generateAppBicep(appBicepPath); err != nil {
		return nil, fmt.Errorf("generating app.bicep: %w", err)
	}
	result.AppBicepPath = appBicepPath
	result.CreatedFiles = append(result.CreatedFiles, "app.bicep")

	// Generate template-specific files
	if s.opts.Template != "" {
		files, err := s.generateTemplateFiles()
		if err != nil {
			return nil, fmt.Errorf("generating template files: %w", err)
		}
		result.CreatedFiles = append(result.CreatedFiles, files...)
	}

	// Generate team practices config if available
	if s.practices != nil {
		practicesPath := filepath.Join(radiusDir, "team-practices.yaml")
		practicesCfg := &practices.PracticesConfig{
			Version:   "1.0",
			Practices: *s.practices,
		}
		if err := practices.SaveConfig(practicesCfg, practicesPath); err != nil {
			return nil, fmt.Errorf("saving team practices: %w", err)
		}
		result.CreatedFiles = append(result.CreatedFiles, ".radius/team-practices.yaml")
	}

	// Create rad.yaml configuration
	radYamlPath := filepath.Join(radiusDir, "rad.yaml")
	if err := s.generateRadYaml(radYamlPath); err != nil {
		return nil, fmt.Errorf("generating rad.yaml: %w", err)
	}
	result.CreatedFiles = append(result.CreatedFiles, ".radius/rad.yaml")

	return result, nil
}

func (s *Scaffolder) generateAppBicep(path string) error {
	data := &appBicepData{
		Name:         s.opts.Name,
		Environment:  s.opts.Environment,
		Dependencies: s.opts.Dependencies,
		Practices:    s.practices,
	}

	tmpl, err := template.New("app.bicep").Parse(appBicepTemplate)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	return nil
}

func (s *Scaffolder) generateTemplateFiles() ([]string, error) {
	var files []string

	switch strings.ToLower(s.opts.Template) {
	case "web-api":
		f, err := s.generateWebAPIFiles()
		if err != nil {
			return nil, err
		}
		files = append(files, f...)

	case "worker":
		f, err := s.generateWorkerFiles()
		if err != nil {
			return nil, err
		}
		files = append(files, f...)

	case "frontend":
		f, err := s.generateFrontendFiles()
		if err != nil {
			return nil, err
		}
		files = append(files, f...)

	default:
		// Unknown template, skip
	}

	return files, nil
}

func (s *Scaffolder) generateWebAPIFiles() ([]string, error) {
	var files []string

	// Create Dockerfile for web API
	dockerfilePath := filepath.Join(s.opts.OutputPath, "Dockerfile")
	content := `# Dockerfile for web API
# Update this with your specific build steps

FROM node:18-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM node:18-alpine
WORKDIR /app
COPY --from=build /app/dist ./dist
COPY --from=build /app/node_modules ./node_modules
EXPOSE 3000
CMD ["node", "dist/main.js"]
`
	if err := os.WriteFile(dockerfilePath, []byte(content), 0644); err != nil {
		return nil, err
	}
	files = append(files, "Dockerfile")

	return files, nil
}

func (s *Scaffolder) generateWorkerFiles() ([]string, error) {
	var files []string

	// Create Dockerfile for worker
	dockerfilePath := filepath.Join(s.opts.OutputPath, "Dockerfile")
	content := `# Dockerfile for worker service
# Update this with your specific build steps

FROM python:3.11-slim AS build
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .

FROM python:3.11-slim
WORKDIR /app
COPY --from=build /app .
CMD ["python", "worker.py"]
`
	if err := os.WriteFile(dockerfilePath, []byte(content), 0644); err != nil {
		return nil, err
	}
	files = append(files, "Dockerfile")

	return files, nil
}

func (s *Scaffolder) generateFrontendFiles() ([]string, error) {
	var files []string

	// Create Dockerfile for frontend
	dockerfilePath := filepath.Join(s.opts.OutputPath, "Dockerfile")
	content := `# Dockerfile for frontend application
# Update this with your specific build steps

FROM node:18-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`
	if err := os.WriteFile(dockerfilePath, []byte(content), 0644); err != nil {
		return nil, err
	}
	files = append(files, "Dockerfile")

	return files, nil
}

func (s *Scaffolder) generateRadYaml(path string) error {
	content := fmt.Sprintf(`# Radius application configuration
application: %s
environment: %s
`, s.opts.Name, s.opts.Environment)

	return os.WriteFile(path, []byte(content), 0644)
}

type appBicepData struct {
	Name         string
	Environment  string
	Dependencies []string
	Practices    *practices.TeamPractices
}

const appBicepTemplate = `// ============================================================================
// Application: {{ .Name }}
// Generated by 'rad app scaffold'
//
// Review this file and customize as needed before deploying.
// See https://docs.radapp.io for more information.
// ============================================================================

extension radius

@description('The Radius environment to deploy to')
param environment string

@description('The application name')
param applicationName string = '{{ .Name }}'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: applicationName
  properties: {
    environment: environment
  }
}
{{ range .Dependencies }}
// TODO: Configure {{ . }} resource
resource {{ . }} 'Applications.Datastores/{{ . }}Databases@2023-10-01-preview' = {
  name: '{{ . }}'
  properties: {
    application: app.id
    environment: environment
  }
}
{{ end }}
// TODO: Add your container resources here
// resource container 'Applications.Core/containers@2023-10-01-preview' = {
//   name: 'mycontainer'
//   properties: {
//     application: app.id
//     container: {
//       image: 'myimage:latest'
//       ports: {
//         http: {
//           containerPort: 3000
//         }
//       }
//     }
//   }
// }
`
