package sd

import (
	"time"
	"net/url"
)

type ServiceDiscovery interface{
	Discovery(meta []byte, interval time.Duration) <-chan url.URL
}
