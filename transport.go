package statsig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxRetries        = 5
	backoffMultiplier = 10
)

type transport struct {
	api       string
	sdkKey    string
	metadata  statsigMetadata // Safe to read from but not thread safe to write into. If value needs to change, please ensure thread safety.
	client    *http.Client
	options   *Options
	sessionID string
}

func getSessionID() string {
	return uuid.NewString()
}

func newTransport(secret string, options *Options) *transport {
	api := defaultString(options.API, DefaultEndpoint)
	api = strings.TrimSuffix(api, "/")
	defer func() {
		if err := recover(); err != nil {
			global.Logger().LogError(err)
		}
	}()
	sid := getSessionID()

	return &transport{
		api:       api,
		metadata:  getStatsigMetadata(),
		sdkKey:    secret,
		client:    &http.Client{Timeout: time.Second * 3},
		options:   options,
		sessionID: sid,
	}
}

func (transport *transport) postRequest(
	endpoint string,
	in interface{},
	out interface{},
) (*http.Response, error) {
	return transport.postRequestInternal(endpoint, in, out, 0, 0)
}

func (transport *transport) retryablePostRequest(
	endpoint string,
	in interface{},
	out interface{},
	retries int,
) (*http.Response, error) {
	return transport.postRequestInternal(endpoint, in, out, retries, time.Second)
}

func (transport *transport) postRequestInternal(
	endpoint string,
	in interface{},
	out interface{},
	retries int,
	backoff time.Duration,
) (*http.Response, error) {
	if transport.options.LocalMode {
		return nil, nil
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}

	return retry(retries, time.Duration(backoff), func() (*http.Response, bool, error) {
		response, err := transport.doRequest(endpoint, body)
		if err != nil {
			return response, response != nil, err
		}
		defer response.Body.Close()

		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return response, false, json.NewDecoder(response.Body).Decode(&out)
		}

		return response, shouldRetry(response.StatusCode), fmt.Errorf("http response error code: %d", response.StatusCode)
	})
}

func (transport *transport) doRequest(endpoint string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", transport.api+endpoint, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Add("STATSIG-API-KEY", transport.sdkKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Add("STATSIG-CLIENT-TIME", strconv.FormatInt(getUnixMilli(), 10))
	req.Header.Add("STATSIG-SERVER-SESSION-ID", transport.sessionID)
	req.Header.Add("STATSIG-SDK-TYPE", transport.metadata.SDKType)
	req.Header.Add("STATSIG-SDK-VERSION", transport.metadata.SDKVersion)

	return transport.client.Do(req)
}

func (transport *transport) get(url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return transport.client.Do(req)
}

func retry(retries int, backoff time.Duration, fn func() (*http.Response, bool, error)) (*http.Response, error) {
	for {
		if response, retry, err := fn(); retry {
			if retries <= 0 {
				return response, err
			}

			retries--
			time.Sleep(backoff)
			backoff = backoff * backoffMultiplier
		} else {
			return response, err
		}
	}
}

func shouldRetry(code int) bool {
	switch code {
	case 408, 500, 502, 503, 504, 522, 524, 599:
		return true
	default:
		return false
	}
}
