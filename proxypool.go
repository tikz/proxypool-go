package proxypool

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"h12.io/socks"
)

// Proxy represents a single SOCKS 4/5 proxy
type Proxy struct {
	URL         string
	Alive       bool
	protocol    string
	ip          string
	port        int
	lastRequest time.Time
	dial        func(string, string) (net.Conn, error)
	transport   *http.Transport
	client      *http.Client
}

// ProxyPool manages a group of proxies.
// RateLimit is how many seconds to wait between requests, per proxy.
// RetestDelay is after how many seconds should a proxy be retested if its is unavailable (manually call Pool.Test()).
type ProxyPool struct {
	proxies        []*Proxy
	TestURL        string
	RateLimit      int
	RetestDelay    int
	AliveCount     int
	AvailableCount int
}

// Create checks if its v4 or v5, constructs the dial, transport, client and finally tests it.
func (proxy *Proxy) Create(testURL string, wg *sync.WaitGroup) {
	socksVersions := [2]int{5, 4}
	for _, version := range socksVersions {
		proxy.URL = fmt.Sprintf("socks%d://%s:%d", version, proxy.ip, proxy.port)
		proxy.dial = socks.Dial(proxy.URL + "?timeout=20s")
		proxy.transport = &http.Transport{
			Dial:              proxy.dial,
			DisableKeepAlives: true,
		}
		proxy.client = &http.Client{Transport: proxy.transport, Timeout: 20 * time.Second}
		_, err := proxy.Get(testURL)
		if err == nil {
			break
		}
	}
	wg.Done()
}

// Get fetchs an URL with the given proxy and returns the body text
func (proxy *Proxy) Get(url string) ([]byte, error) {
	proxy.lastRequest = time.Now()
	resp, err := proxy.client.Get(url)
	if err != nil {
		proxy.Alive = false
		fmt.Println(err)
		return nil, fmt.Errorf("HTTP request failed: %s", err)
	}
	defer resp.Body.Close()

	buf, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		proxy.Alive = false
		return nil, fmt.Errorf("can't read response: %s", err)
	}

	if resp.StatusCode != http.StatusOK {
		proxy.Alive = false
		return nil, fmt.Errorf("test URL replied with HTTP code %d", resp.StatusCode)
	}
	proxy.Alive = true
	return buf, nil
}

// GetAvailableProxy returns an available proxy from the pool.
func (pool *ProxyPool) GetAvailableProxy() (*Proxy, error) {
	for i := range pool.proxies {
		if time.Since(pool.proxies[i].lastRequest).Seconds() > float64(pool.RetestDelay) && !pool.proxies[i].Alive {
			go pool.proxies[i].Get(pool.TestURL)
		}
		if time.Since(pool.proxies[i].lastRequest).Seconds() > float64(pool.RateLimit) && pool.proxies[i].Alive {
			return pool.proxies[i], nil
		}
	}
	return nil, errors.New("no proxies available")
}

// NewProxyPool constructs a new ProxyPool instance
func NewProxyPool(testURL string, rateLimit int, retestDelay int) *ProxyPool {
	pool := &ProxyPool{TestURL: testURL, RateLimit: rateLimit, RetestDelay: retestDelay}
	return pool
}

// LoadProxies loads the pool with SOCKS4/5 proxies from a text file
func (pool *ProxyPool) LoadProxies(path string) error {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var proxies []*Proxy
	scanner := bufio.NewScanner(file)
	var wg sync.WaitGroup
	for scanner.Scan() {
		line := strings.Split(scanner.Text(), ":")
		ip := line[0]
		port, _ := strconv.Atoi(line[1])
		proxy := Proxy{ip: ip, port: port}
		proxies = append(proxies, &proxy)

		wg.Add(1)
		go proxy.Create(pool.TestURL, &wg)
	}
	wg.Wait()

	for _, p := range proxies {
		if p.Alive {
			pool.proxies = append(pool.proxies, p)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	pool.UpdateCounts()
	return nil
}

// Get waits until a proxy from the pool is available and then fetchs the given URL.
func (pool *ProxyPool) Get(url string) []byte {
	for {
		proxy, err := pool.GetAvailableProxy()
		if err == nil {
			r, reqErr := proxy.Get(url)
			if reqErr == nil {
				return r
			}
		}
		time.Sleep(time.Second)
	}
}

// UpdateCounts updates the pool counters
func (pool *ProxyPool) UpdateCounts() {
	var alive, available int
	for i := range pool.proxies {
		if pool.proxies[i].Alive {
			alive++
			if time.Since(pool.proxies[i].lastRequest).Seconds() > float64(pool.RateLimit) {
				available++
			}
		}
	}
	pool.AliveCount = alive
	pool.AvailableCount = available
}
