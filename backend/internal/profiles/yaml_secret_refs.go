package profiles

import (
	"strings"

	"gopkg.in/yaml.v3"
)

type profileYAMLDocument struct {
	SecretRefs []SecretReference `yaml:"secretRefs"`
}

func ExtractSecretRefsFromYAML(source string) []SecretReference {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}

	var document profileYAMLDocument
	if err := yaml.Unmarshal([]byte(source), &document); err != nil {
		return nil
	}

	return compactSecretRefs(document.SecretRefs)
}

func mergeSecretRefs(base []SecretReference, overlays ...[]SecretReference) []SecretReference {
	total := len(base)
	for _, overlay := range overlays {
		total += len(overlay)
	}
	if total == 0 {
		return nil
	}

	ordered := make([]SecretReference, 0, total)
	indexByKey := make(map[string]int, total)
	appendRef := func(ref SecretReference) {
		key := strings.TrimSpace(ref.Key)
		if key == "" {
			return
		}
		if index, ok := indexByKey[key]; ok {
			ordered[index] = ref
			return
		}
		indexByKey[key] = len(ordered)
		ordered = append(ordered, ref)
	}

	for _, ref := range compactSecretRefs(base) {
		appendRef(ref)
	}
	for _, overlay := range overlays {
		for _, ref := range compactSecretRefs(overlay) {
			appendRef(ref)
		}
	}
	if len(ordered) == 0 {
		return nil
	}
	return ordered
}
