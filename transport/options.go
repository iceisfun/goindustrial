package transport

// Config holds lifecycle hooks shared by all transport implementations.
type Config struct {
	OnConnect    func()
	OnDisconnect func(error)
}

// Option configures transport lifecycle hooks.
type Option func(*Config)

// WithOnConnect registers a callback that fires after a connection is established.
func WithOnConnect(fn func()) Option {
	return func(cfg *Config) {
		cfg.OnConnect = fn
	}
}

// WithOnDisconnect registers a callback that fires when a connection is lost.
func WithOnDisconnect(fn func(error)) Option {
	return func(cfg *Config) {
		cfg.OnDisconnect = fn
	}
}

func applyOptions(opts []Option) Config {
	var cfg Config
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}
