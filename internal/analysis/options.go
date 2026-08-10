package analysis

type Options struct {
	RootPath     string
	OutputDir    string
	Addons       []string
	IncludeTests bool
}

type ProgressFunc func(phase string, counts map[string]int)
