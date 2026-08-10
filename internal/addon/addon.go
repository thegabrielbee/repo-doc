package addon

import "github.com/bee/java-process-mapper/internal/model"

type Addon interface {
	Name() string
	Analyze(project *model.Project) error
}

func Resolve(names []string) []Addon {
	if len(names) == 0 {
		names = []string{"spring"}
	}
	var addons []Addon
	for _, name := range names {
		switch name {
		case "spring":
			addons = append(addons, Spring{})
		}
	}
	return addons
}
