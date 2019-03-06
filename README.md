# proxypool-go

Proxypool is a wrapper around the h12.io/socks library that implements a pool of rotating SOCKS4/5 proxies for making HTTP requests. It is also safe to use with goroutines.


# Example
```go
package main

import (
	"fmt"
	"log"
	"proxypool"
)

func main() {
	testURL := "http://example.com" // The URL used for testing all proxies (expects 200 code)
	rateLimit := 10                 // How much seconds to wait between requests, per proxy
	retestDelay := 300              // How much seconds to wait to retest a proxy that became down

	pool := proxypool.NewProxyPool(testURL, rateLimit, retestDelay)

	// Load proxies from file, expects ip:port per line
	pool.LoadProxies("./proxies.txt")

	// pool.Get() waits for a proxy from the pool to become available and retrying forever
	HTML := pool.Get("http://example.com/something")
	fmt.Println(HTML)

	// Also you can manually use:
	proxy, err := pool.GetAvailableProxy()
	if err != nil {
		log.Fatal(err)
	}
	proxy.Get("http://example.com/something")
}
```