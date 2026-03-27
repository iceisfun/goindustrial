package transport

// Config holds lifecycle hooks shared by all transport implementations.
type Config struct {
	// OnConnect callbacks fire after a new connection is successfully
	// established. Multiple callbacks may be registered and they execute
	// in registration order.
	OnConnect []func()

	// OnDisconnect callbacks fire when a connection is lost or closed.
	// The error parameter is the result of the underlying Close call.
	// Multiple callbacks may be registered and they execute in
	// registration order.
	OnDisconnect []func(error)
}

// Option configures transport lifecycle hooks.
type Option func(*Config)

// WithOnConnect registers a callback that fires after a connection is
// established. It may be called multiple times to register additional
// callbacks; they execute in registration order.
func WithOnConnect(fn func()) Option {
	return func(cfg *Config) {
		cfg.OnConnect = append(cfg.OnConnect, fn)
	}
}

// WithOnDisconnect registers a callback that fires when a connection is lost.
// It may be called multiple times to register additional callbacks; they
// execute in registration order.
func WithOnDisconnect(fn func(error)) Option {
	return func(cfg *Config) {
		cfg.OnDisconnect = append(cfg.OnDisconnect, fn)
	}
}

func (c *Config) fireOnConnect() {
	for _, fn := range c.OnConnect {
		fn()
	}
}

func (c *Config) fireOnDisconnect(err error) {
	for _, fn := range c.OnDisconnect {
		fn(err)
	}
}

func applyOptions(opts []Option) Config {
	var cfg Config
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}
