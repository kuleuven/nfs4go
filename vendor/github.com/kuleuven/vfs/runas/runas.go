package runas

func RunAsCurrentUser() Context {
	return &current{}
}

type Context interface {
	Run(f func() error) error
	Close() error
}

type current struct{}

func (c *current) Run(f func() error) error {
	return f()
}

func (c *current) Close() error {
	return nil
}
