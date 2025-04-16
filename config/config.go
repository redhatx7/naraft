package config

type Config struct {
	MinElectionTimeout int
	MaxElectionTimeout int
}

func NewDefaultConfig() *Config {
	return &Config{
		MinElectionTimeout: 150,
		MaxElectionTimeout: 300,
	}
}
