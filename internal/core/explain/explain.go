package explain

type Explain interface {
	KV(key string, value any)
	Timer(name string) func()
}

