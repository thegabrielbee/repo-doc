package mappingstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bee/java-process-mapper/internal/model"
)

const (
	Version                  = 1
	StateFileName            = "mapping-state.json"
	FinalDocsDirName         = "features"
	MechanicalDocsDir        = "processes"
	StatusPending     Status = "pending"
	StatusMapped      Status = "mapped"
)

type Status string

type State struct {
	Version   int       `json:"version"`
	OutputDir string    `json:"outputDir"`
	Items     []Item    `json:"items"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Item struct {
	EntryPointID      string    `json:"entryPointId"`
	Status            Status    `json:"status"`
	Name              string    `json:"name"`
	Kind              string    `json:"kind"`
	Path              string    `json:"path,omitempty"`
	HTTPMethod        string    `json:"httpMethod,omitempty"`
	Resource          string    `json:"resource,omitempty"`
	ClassID           string    `json:"classId,omitempty"`
	MethodID          string    `json:"methodId,omitempty"`
	MechanicalDocPath string    `json:"mechanicalDocPath"`
	FinalDocPath      string    `json:"finalDocPath"`
	Title             string    `json:"title,omitempty"`
	Notes             string    `json:"notes,omitempty"`
	MappedAt          time.Time `json:"mappedAt,omitempty"`
}

func StatePath(outputDir string) string {
	return filepath.Join(outputDir, StateFileName)
}

func FinalDocsDir(outputDir string) string {
	return filepath.Join(outputDir, "docs", FinalDocsDirName)
}

func Initialize(outputDir string, project *model.Project) (State, error) {
	state := State{
		Version:   Version,
		OutputDir: filepath.Clean(outputDir),
		Items:     BuildItems(outputDir, project),
		UpdatedAt: time.Now().UTC(),
	}
	if err := os.MkdirAll(FinalDocsDir(outputDir), 0o755); err != nil {
		return State{}, err
	}
	if err := Save(StatePath(outputDir), state); err != nil {
		return State{}, err
	}
	return state, nil
}

func BuildItems(outputDir string, project *model.Project) []Item {
	if project == nil {
		return nil
	}
	entries := append([]model.EntryPoint(nil), project.EntryPoints...)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].ID < entries[j].ID
	})

	docsDir := filepath.Join(outputDir, "docs")
	processesDir := filepath.Join(docsDir, MechanicalDocsDir)
	featuresDir := filepath.Join(docsDir, FinalDocsDirName)
	seenDocNames := map[string]int{}
	items := make([]Item, 0, len(entries))
	for _, entry := range entries {
		baseName := SafeFileName(entry.Kind + "-" + entry.Name)
		if baseName == "" {
			baseName = SafeFileName(entry.ID)
		}
		name := baseName
		if seenDocNames[baseName] > 0 {
			name = baseName + "-" + strconv.Itoa(seenDocNames[baseName]+1)
		}
		seenDocNames[baseName]++
		items = append(items, Item{
			EntryPointID:      entry.ID,
			Status:            StatusPending,
			Name:              entry.Name,
			Kind:              entry.Kind,
			Path:              entry.Path,
			HTTPMethod:        entry.HTTPMethod,
			Resource:          entry.Resource,
			ClassID:           entry.ClassID,
			MethodID:          entry.MethodID,
			MechanicalDocPath: filepath.Join(processesDir, name+".md"),
			FinalDocPath:      filepath.Join(featuresDir, name+".md"),
		})
	}
	return items
}

func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func Save(path string, state State) error {
	state.Version = Version
	state.OutputDir = filepath.Clean(state.OutputDir)
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Counts(state State) (total, mapped, remaining int) {
	total = len(state.Items)
	for _, item := range state.Items {
		if item.Status == StatusMapped {
			mapped++
			continue
		}
		remaining++
	}
	return total, mapped, remaining
}

func Next(state State) (Item, bool) {
	for _, item := range state.Items {
		if item.Status != StatusMapped {
			return item, true
		}
	}
	return Item{}, false
}

func SafeFileName(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		isAlphaNum := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	value = strings.Trim(b.String(), "-")
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	if len(value) > 120 {
		value = value[:120]
	}
	return value
}
