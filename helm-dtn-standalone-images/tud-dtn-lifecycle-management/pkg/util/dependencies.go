package util

type Dependencies struct {
	Images map[string][]string `yaml:"images"`
	Git    map[string]string   `yaml:"git"`
	// TODO: extend with whatever else we need to fetch
}
