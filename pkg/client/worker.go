package client

// NewWorkerClient creates a Client that routes requests through the VOT worker
// proxy (useful when the Yandex API is geo-blocked). Bodies are wrapped in a
// JSON envelope and sent to the worker host.
func NewWorkerClient(opts Options) (*Client, error) {
	opts.Worker = true
	return New(opts)
}
