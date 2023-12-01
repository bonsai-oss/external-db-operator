package database

type Type string

type Provider interface {
	Initialize(dsn string) error
	Apply(options CreateOptions) error
	Destroy(options DestroyOptions) error
	GetDSN() string
}

type CreateOptions struct {
	Name     string
	Password string
}

type DestroyOptions struct {
	Name string
}

var registeredProviders = map[string]ProviderInitializer{}

type ProviderInitializer func() Provider

func RegisterProvider(name string, initializer ProviderInitializer) {
	registeredProviders[name] = initializer
}

func Provide(name string) Provider {
	return registeredProviders[name]()
}
