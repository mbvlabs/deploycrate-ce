package inertia

import gonertia "github.com/romsar/gonertia/v3"

func WithSharedProp(key string, value any) Option {
	return func(i *gonertia.Inertia) error {
		i.ShareProp(key, value)
		return nil
	}
}
