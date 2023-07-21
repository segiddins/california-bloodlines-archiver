package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sync/semaphore"
)

var httpClient = &http.Client{
	Transport: &httpCache{
		RoundTripper: http.DefaultTransport,
	},
}

type httpCache struct {
	lock      sync.RWMutex
	connCount map[string]*semaphore.Weighted
	http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (c *httpCache) RoundTrip(req *http.Request) (*http.Response, error) {
	hash, err := c.hashRequest(req)
	if err != nil {
		return nil, err
	}

	if resp, err := c.cachedResponse(hash, req); err == nil {
		return resp, nil
	}

	if err := c.acquireConn(req.Host); err != nil {
		return nil, err
	}
	resp, err := c.RoundTripper.RoundTrip(req)
	c.hostSema(req.Host).Release(1)
	if err != nil {
		return nil, err
	}
	_ = resp

	c.cacheResponse(resp, hash)

	return c.cachedResponse(hash, req)
}

func (c *httpCache) hostSema(host string) *semaphore.Weighted {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.connCount == nil {
		c.connCount = map[string]*semaphore.Weighted{}
	}

	if sema, ok := c.connCount[host]; ok {
		return sema
	}

	sema := semaphore.NewWeighted(10)
	c.connCount[host] = sema
	return sema
}

func (c *httpCache) acquireConn(host string) error {
	return c.hostSema(host).Acquire(context.TODO(), 1)
}

func (c *httpCache) cacheResponse(resp *http.Response, hash string) {
	var buf bytes.Buffer

	if err := resp.Write(&buf); err != nil {
		panic(err)
	}

	b, _ := io.ReadAll(bufio.NewReader(&buf))

	c.lock.Lock()
	defer c.lock.Unlock()

	os.WriteFile("httpcache/"+hash, b, 0644)
}

func (c *httpCache) cachedResponse(hash string, req *http.Request) (*http.Response, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if resp, err := os.ReadFile("httpcache/" + hash + ".resp"); err != os.ErrNotExist {
		if err != nil {
			return nil, err
		}

		return http.ReadResponse(bufio.NewReader(bytes.NewReader(resp)), req)
	}

	return nil, nil
}

func (c *httpCache) hashRequest(req *http.Request) (string, error) {
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return "", err
		}

		req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		defer func() {
			req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
		}()
	}
	buf := &bytes.Buffer{}

	if err := req.Write(buf); err != nil {
		return "", err
	}

	reqBytes := buf.Bytes()

	digest := sha256.New()
	if _, err := digest.Write(reqBytes); err != nil {
		return "", err
	}

	sum := digest.Sum([]byte{})

	str := hex.EncodeToString(sum)

	str = filepath.Join(req.Host, req.URL.RequestURI(), req.Method, str)

	c.writeRequest(str, buf)

	return str, nil
}

func (c *httpCache) writeRequest(sum string, req *bytes.Buffer) {
	path := "httpcache/" + sum + ".req"
	os.MkdirAll(filepath.Dir(path), 0755)
	if f, err := os.OpenFile(path, os.O_RDONLY, 0644); err == nil {
		f.Close()
		return
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	os.WriteFile(path, req.Bytes(), 0644)
}

var _ http.RoundTripper = &httpCache{}
