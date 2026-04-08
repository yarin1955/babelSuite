package suites

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/babelsuite/babelsuite/internal/examplefs"
	"gopkg.in/yaml.v3"
)

type workspaceProfileDocument struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Default     bool   `yaml:"default"`
	Runtime     struct {
		Suite       string `yaml:"suite"`
		Repository  string `yaml:"repository"`
		ProfileFile string `yaml:"profileFile"`
	} `yaml:"runtime"`
	Modules []string `yaml:"modules"`
}

func loadWorkspaceSuites() map[string]Definition {
	root := filepath.Join(examplefs.ResolveRoot(), "oci-suites")
	entries, err := os.ReadDir(root)
	if err != nil {
		return map[string]Definition{}
	}

	result := make(map[string]Definition, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		definition, ok := loadWorkspaceSuite(root, entry.Name())
		if !ok {
			continue
		}
		result[definition.ID] = definition
	}
	return result
}

func loadWorkspaceSuite(root, suiteID string) (Definition, bool) {
	base := filepath.Join(root, suiteID)
	suiteStarBytes, err := os.ReadFile(filepath.Join(base, "suite.star"))
	if err != nil {
		return Definition{}, false
	}

	profiles, repository, modules := loadWorkspaceProfiles(base)
	title, description := loadWorkspaceReadme(base, suiteID)
	rootSources := loadWorkspaceRootSourceFiles(base)
	if repository == "" {
		repository = "workspace/" + suiteID
	}

	definition := Definition{
		ID:          suiteID,
		Title:       title,
		Repository:  repository,
		Owner:       ownerFromRepository(repository),
		Provider:    "Workspace",
		Version:     "workspace",
		Tags:        []string{"workspace"},
		Description: description,
		Modules:     modules,
		Status:      "Installed",
		Score:       0,
		PullCommand: workspaceRunCommand(repository),
		ForkCommand: workspaceForkCommand(repository),
		SuiteStar:   string(suiteStarBytes),
		Profiles:    profiles,
		Folders:     loadWorkspaceFolders(base),
		SeedSources: rootSources,
		Contracts:   loadWorkspaceContracts(string(suiteStarBytes)),
	}

	return definition, true
}

func loadWorkspaceProfiles(base string) ([]ProfileOption, string, []string) {
	dir := filepath.Join(base, "profiles")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", nil
	}

	profiles := make([]ProfileOption, 0, len(entries))
	moduleSet := map[string]struct{}{}
	repository := ""
	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var profile workspaceProfileDocument
		if err := yaml.Unmarshal(data, &profile); err != nil {
			continue
		}

		fileName := firstWorkspaceNonEmpty(strings.TrimSpace(profile.Runtime.ProfileFile), entry.Name())
		profiles = append(profiles, ProfileOption{
			FileName:    fileName,
			Label:       firstWorkspaceNonEmpty(strings.TrimSpace(profile.Name), workspaceLabelFromFileName(fileName)),
			Description: strings.TrimSpace(profile.Description),
			Default:     profile.Default,
		})

		if repository == "" {
			repository = strings.TrimSpace(profile.Runtime.Repository)
		}
		for _, module := range profile.Modules {
			trimmed := strings.TrimSpace(module)
			if trimmed == "" {
				continue
			}
			moduleSet[trimmed] = struct{}{}
		}
	}

	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].Default != profiles[j].Default {
			return profiles[i].Default
		}
		return profiles[i].FileName < profiles[j].FileName
	})

	modules := make([]string, 0, len(moduleSet))
	for module := range moduleSet {
		modules = append(modules, module)
	}
	sort.Strings(modules)

	return profiles, repository, modules
}

func loadWorkspaceReadme(base, suiteID string) (string, string) {
	data, err := os.ReadFile(filepath.Join(base, "README.md"))
	if err != nil {
		title := humanizeIdentifier(suiteID)
		return title, title + " installed from the local workspace."
	}

	lines := strings.Split(string(data), "\n")
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts = append(parts, trimmed)
	}
	title := humanizeIdentifier(suiteID)
	if len(parts) > 0 {
		title = parts[0]
	}
	description := title + " installed from the local workspace."
	if len(parts) > 1 {
		description = parts[1]
	}
	return title, description
}

func loadWorkspaceFolders(base string) []FolderEntry {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}

	folders := make([]FolderEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		files := make([]string, 0)
		folderRoot := filepath.Join(base, entry.Name())
		_ = filepath.WalkDir(folderRoot, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(folderRoot, path)
			if err != nil {
				return nil
			}
			files = append(files, filepath.ToSlash(relative))
			return nil
		})
		sort.Strings(files)

		folders = append(folders, FolderEntry{
			Name:        entry.Name(),
			Role:        folderRole(entry.Name()),
			Description: folderDescription(entry.Name()),
			Files:       files,
		})
	}

	sort.Slice(folders, func(i, j int) bool {
		return folders[i].Name < folders[j].Name
	})
	return folders
}

func loadWorkspaceRootSourceFiles(base string) []SourceFile {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}

	files := make([]SourceFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.EqualFold(name, "suite.star") || strings.EqualFold(name, "README.md") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(base, name))
		if err != nil {
			continue
		}
		files = append(files, SourceFile{
			Path:     name,
			Language: detectSourceLanguage(name),
			Content:  string(content),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

func loadWorkspaceContracts(suiteStar string) []string {
	lines := strings.Split(suiteStar, "\n")
	contracts := make([]string, 0)
	seen := map[string]struct{}{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "load(") {
			continue
		}
		start := strings.Index(line, "\"")
		if start == -1 {
			continue
		}
		rest := line[start+1:]
		end := strings.Index(rest, "\"")
		if end == -1 {
			continue
		}
		module := strings.TrimSpace(rest[:end])
		if module == "" {
			continue
		}
		if _, ok := seen[module]; ok {
			continue
		}
		seen[module] = struct{}{}
		contracts = append(contracts, module)
	}
	return contracts
}

func folderRole(name string) string {
	switch strings.TrimSpace(name) {
	case "profiles":
		return "Configuration"
	case "api":
		return "Contracts"
	case "mock":
		return "Mocking"
	case "scripts":
		return "Setup"
	case "scenarios":
		return "Validation"
	case "fixtures":
		return "Data"
	case "policies":
		return "Policy"
	default:
		return humanizeIdentifier(name)
	}
}

func folderDescription(name string) string {
	switch strings.TrimSpace(name) {
	case "profiles":
		return "Environment-specific runtime overrides and launch profiles."
	case "api":
		return "Contracts and schemas shipped with the suite."
	case "mock":
		return "Mock schemas, metadata, and compatibility assets."
	case "scripts":
		return "Short-lived setup and migration scripts."
	case "scenarios":
		return "Executable smoke, regression, and attack-path tests."
	case "fixtures":
		return "Static seed data and sample payloads."
	case "policies":
		return "Policy rules and invariants enforced by the suite."
	default:
		return humanizeIdentifier(name) + " files."
	}
}

func ownerFromRepository(repository string) string {
	trimmed := strings.TrimSpace(repository)
	if trimmed == "" {
		return "Workspace"
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return "Workspace"
	}
	return humanizeIdentifier(parts[len(parts)-2])
}

func humanizeIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Workspace"
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '-' || r == '_' || r == '/' || r == '.'
	})
	for index := range parts {
		if parts[index] == "" {
			continue
		}
		parts[index] = strings.ToUpper(parts[index][:1]) + strings.ToLower(parts[index][1:])
	}
	return strings.Join(parts, " ")
}

func isYAMLFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(name)))
	return ext == ".yaml" || ext == ".yml"
}

func firstWorkspaceNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func workspaceLabelFromFileName(fileName string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(fileName), filepath.Ext(strings.TrimSpace(fileName)))
	return humanizeIdentifier(trimmed)
}

func workspaceRunCommand(repository string) string {
	version := "workspace"
	return "babelctl run " + strings.TrimSpace(repository) + ":" + version
}

func workspaceForkCommand(repository string) string {
	version := "workspace"
	name := strings.TrimSpace(repository)
	if parts := strings.Split(strings.Trim(name, "/"), "/"); len(parts) > 0 {
		name = parts[len(parts)-1]
	}
	return "babelctl fork " + strings.TrimSpace(repository) + ":" + version + " ./" + name
}
