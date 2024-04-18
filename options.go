package mini

import "github.com/nats-io/nats.go"

type Options struct {
	Path          string
	NatsUrl       string
	NatsOptions   []nats.Option
	LogLevel      string
	Endpoints     []EndpointInitializer
	ConfigWatched bool
}

type Option func(*Options) error

func WithConfigWatched() Option {
	return func(o *Options) error {
		o.ConfigWatched = true
		return nil
	}
}

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

func WithPath(path string) Option {
	return func(o *Options) error {
		o.Path = path
		return nil
	}
}

func WithCredentials(jwt string, seed string) Option {
	return func(o *Options) error {
		o.NatsOptions = append(o.NatsOptions, nats.UserJWTAndSeed(jwt, seed))
		return nil
	}
}

func WithCredentialsFile(file string) Option {
	return func(o *Options) error {
		o.NatsOptions = append(o.NatsOptions, nats.UserCredentials(file))
		return nil
	}
}
