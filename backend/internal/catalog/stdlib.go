package catalog

import "github.com/babelsuite/babelsuite/internal/demofs"

func SeedStdlibPackages() []Package {
	return clonePackages(seedStdlib())
}

func seedStdlib() []Package {
	manifest, err := demofs.LoadManifest()
	if err != nil {
		return nil
	}

	packages, err := demofs.LoadJSON[[]Package](manifest.StdlibFile)
	if err != nil {
		return nil
	}

	return clonePackages(packages)
}
