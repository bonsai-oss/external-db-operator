package database

import (
	"fmt"
	"io"
)

type Type string

type Provider interface {
	Initialize(dsn string) error
	Apply(options CreateOptions) error
	Destroy(options DestroyOptions) error
	GetDSN() string
	Ping() error
	io.Closer
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

type ErrUnknownProvider struct {
	Name string
}

func (e ErrUnknownProvider) Error() string {
	return fmt.Sprintf("unknown provider: %s", e.Name)
}

func Provide(name string) (Provider, error) {
	providerInitializer, found := registeredProviders[name]
	if !found {
		return nil, ErrUnknownProvider{Name: name}
	}

	return providerInitializer(), nil
}
