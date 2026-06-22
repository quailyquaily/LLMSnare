package stockvault

type Transport struct {
	TimeoutSeconds int
}

func DefaultTransport() Transport {
	return Transport{TimeoutSeconds: 30}
}
