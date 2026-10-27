package util

type Config struct {
	DestinationEid string `yaml:"destinationEid"`
	Registry       string `yaml:"registry"`
	Socket         string `yaml:"socket"`
	Tcp            string `yaml:"tcp"`
}
