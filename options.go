package mini

import "github.com/nats-io/nats.go"

type Options struct {
	Name        string
	Description string
	Version     string
	Account     string
	NatsUrl     string
	NatsOptions []nats.Option
	LogLevel    string
	Endpoints   []EndpointInitializer
}

type Option func(*Options) error

func WithLogLevel(level string) Option {
	return func(o *Options) error {
		o.LogLevel = level
		return nil
	}
}

func WithNatsUrl(url string) Option {
	return func(o *Options) error {
		o.NatsUrl = url
		return nil
	}
}

func WithEndpoints(endpoints ...EndpointInitializer) Option {
	return func(o *Options) error {
		o.Endpoints = append(o.Endpoints, endpoints...)
		return nil
	}
}

func WithAccount(account string) Option {
	return func(o *Options) error {
		o.Account = account
		return nil
	}
}

func WithName(name string) Option {
	return func(o *Options) error {
		o.Name = name
		return nil
	}
}

func WithDescription(desc string) Option {
	return func(o *Options) error {
		o.Description = desc
		return nil
	}
}

func WithVersion(version string) Option {
	return func(o *Options) error {
		o.Version = version
		return nil
	}
}

func WithCredentials(jwt string, seed string) Option {
	return func(o *Options) error {
		o.NatsOptions = append(o.NatsOptions, nats.UserCredentials(jwt, seed))
		return nil
	}
}
