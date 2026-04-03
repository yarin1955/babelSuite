package profiles

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func applyUpsertRequest(record *Record, request UpsertRequest, current []Record, currentID string) error {
	record.Name = strings.TrimSpace(request.Name)
	record.FileName = strings.TrimSpace(request.FileName)
	record.Description = strings.TrimSpace(request.Description)
	record.Scope = firstNonEmpty(strings.TrimSpace(request.Scope), scopeFromFileName(request.FileName))
	record.YAML = strings.TrimSpace(request.YAML)
	record.SecretRefs = compactSecretRefs(request.SecretRefs)
	record.Default = request.Default && record.Launchable
	record.ExtendsID = strings.TrimSpace(request.ExtendsID)
	record.UpdatedAt = time.Now().UTC()

	if record.ExtendsID == "" && record.Launchable {
		record.ExtendsID = "base"
	}
	if !record.Launchable {
		record.Default = false
	}

	return validateRecord(*record, current, currentID)
}

func validateRecord(record Record, current []Record, currentID string) error {
	if record.Name == "" {
		return errors.New("Profile name is required.")
	}
	if record.FileName == "" {
		return errors.New("Profile file name is required.")
	}
	if !strings.HasSuffix(strings.ToLower(record.FileName), ".yaml") && !strings.HasSuffix(strings.ToLower(record.FileName), ".yml") {
		return errors.New("Profile file names must end with .yaml or .yml.")
	}
	if record.YAML == "" {
		return errors.New("Profile YAML is required.")
	}

	var parsed any
	if err := yaml.Unmarshal([]byte(record.YAML), &parsed); err != nil {
		return fmt.Errorf("Invalid YAML: %v", err)
	}

	for _, existing := range current {
		if existing.ID == currentID {
			continue
		}
		if strings.EqualFold(existing.FileName, record.FileName) {
			return errors.New("A profile with that file name already exists for this suite.")
		}
	}

	if record.ExtendsID != "" && indexOfProfile(current, record.ExtendsID) == -1 && record.ExtendsID != currentID {
		return errors.New("The selected base profile was not found.")
	}

	for _, secretRef := range record.SecretRefs {
		if secretRef.Key == "" || secretRef.Provider == "" || secretRef.Ref == "" {
			return errors.New("Secret references need a key, provider, and reference.")
		}
	}

	return nil
}
