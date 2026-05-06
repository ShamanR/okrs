package auth

import "fmt"

// ProviderFactory creates a Provider from the global Config.
type ProviderFactory func(cfg Config) (Provider, error)

var globalRegistry = map[string]ProviderFactory{}

// Register adds a provider factory to the global registry.
// Call from provider package init() functions.
func Register(name string, factory ProviderFactory) {
	globalRegistry[name] = factory
}

func buildProviders(cfg Config) (map[string]Provider, error) {
	result := map[string]Provider{}
	for _, name := range cfg.EnabledProviders {
		factory, ok := globalRegistry[name]
		if !ok {
			return nil, fmt.Errorf("auth: unknown provider %q (not registered)", name)
		}
		p, err := factory(cfg)
		if err != nil {
			return nil, fmt.Errorf("auth: init provider %q: %w", name, err)
		}
		result[name] = p
	}
	return result, nil
}
