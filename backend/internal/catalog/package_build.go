package catalog

import "strings"

func (s *Service) knownPackages() knownPackageIndex {
	index := knownPackageIndex{
		byFullName: make(map[string]Package, len(s.suites.List())+len(seedStdlib())),
		byPath:     make(map[string]Package, len(s.suites.List())+len(seedStdlib())),
	}

	for _, suite := range s.suites.List() {
		item := Package{
			ID:          suite.ID,
			Kind:        "suite",
			Title:       suite.Title,
			Repository:  suite.Repository,
			Owner:       suite.Owner,
			Provider:    suite.Provider,
			Version:     suite.Version,
			Tags:        append([]string{}, suite.Tags...),
			Description: suite.Description,
			Modules:     append([]string{}, suite.Modules...),
			Status:      suite.Status,
			Score:       suite.Score,
			PullCommand: suite.PullCommand,
			ForkCommand: suite.ForkCommand,
			Inspectable: true,
		}
		index.add(item)
	}

	for _, item := range seedStdlib() {
		clone := item
		clone.Tags = append([]string{}, item.Tags...)
		clone.Modules = append([]string{}, item.Modules...)
		index.add(clone)
	}

	return index
}

func (s *Service) packageForRepository(repo discoveredRepository, known Package) Package {
	provider := firstNonEmpty(strings.TrimSpace(repo.registry.Provider), strings.TrimSpace(repo.registry.Name), "OCI")

	if known.ID != "" {
		item := known
		item.Repository = repo.fullName
		item.Provider = provider
		item.Tags = append([]string{}, repo.tags...)
		item.Version = chooseVersion(repo.tags, known.Version)
		item.PullCommand = buildRunCommand(repo.fullName, item.Version)
		item.ForkCommand = buildForkCommand(repo.fullName, item.Version)
		item.Inspectable = item.Kind == "suite"
		return item
	}

	kind := inferKind(repo.name)
	version := chooseVersion(repo.tags, "")
	return Package{
		ID:          packageID(repo.fullName, kind),
		Kind:        kind,
		Title:       titleForRepository(repo.name, kind),
		Repository:  repo.fullName,
		Owner:       ownerForRepository(repo.name, repo.registry.Name),
		Provider:    provider,
		Version:     version,
		Tags:        append([]string{}, repo.tags...),
		Description: genericDescription(repo.name, repo.registry.Name, kind),
		Modules:     inferModules(repo.name),
		Status:      "Verified",
		Score:       80,
		PullCommand: buildRunCommand(repo.fullName, version),
		ForkCommand: buildForkCommand(repo.fullName, version),
		Inspectable: kind == "suite",
	}
}

func (k knownPackageIndex) add(item Package) {
	fullName := normalizeRepository(item.Repository)
	k.byFullName[fullName] = item

	if repositoryPath := normalizeRepository(repositoryPath(item.Repository)); repositoryPath != "" {
		if _, exists := k.byPath[repositoryPath]; !exists {
			k.byPath[repositoryPath] = item
		}
	}
}

func (k knownPackageIndex) lookup(repo discoveredRepository) Package {
	if item, ok := k.byFullName[normalizeRepository(repo.fullName)]; ok {
		return item
	}
	if item, ok := k.byPath[normalizeRepository(repo.path)]; ok {
		return item
	}
	return Package{}
}

func clonePackages(input []Package) []Package {
	output := make([]Package, len(input))
	for index, item := range input {
		output[index] = item
		output[index].Tags = append([]string{}, item.Tags...)
		output[index].Modules = append([]string{}, item.Modules...)
	}
	return output
}
