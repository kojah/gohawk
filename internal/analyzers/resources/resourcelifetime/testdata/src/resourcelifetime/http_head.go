package resourcelifetime

import (
	"context"
	"net/http"
	"time"
)

func headZeroClient(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	_, err = client.Do(request)
	return err
}

func getZeroClient(url string) error {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headTimeout(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: time.Second}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headCustomTransport(url string, transport http.RoundTripper) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Transport: transport}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headMethodChanged(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	request.Method = "GET"
	client := &http.Client{}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headDefaultClient(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "gohawk")
	_ = request.Header.Get("User-Agent")
	_, err = http.DefaultClient.Do(request)
	return err
}

func headWithContext(ctx context.Context, url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	_, err = client.Do(request.WithContext(ctx))
	return err
}

func headContextConstructor(ctx context.Context, url string) error {
	request, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	_, err = client.Do(request.Clone(ctx))
	return err
}

func getDefaultClient(url string) error {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	_, err = http.DefaultClient.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headDefaultClientTimeoutSet(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	http.DefaultClient.Timeout = time.Second
	_, err = http.DefaultClient.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func configureClient(client *http.Client) {
	client.Timeout = time.Second
}

func headDefaultClientConfiguredByHelper(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	configureClient(http.DefaultClient)
	_, err = http.DefaultClient.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headDefaultTransportReplaced(url string, transport http.RoundTripper) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	http.DefaultTransport = transport
	_, err = http.DefaultClient.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func rewriteMethod(request *http.Request) {
	request.Method = "GET"
}

func headRequestPassedToHelper(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	rewriteMethod(request)
	client := &http.Client{}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headMethodChangedAfterRebinding(ctx context.Context, url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	rebound := request.WithContext(ctx)
	rebound.Method = "GET"
	client := &http.Client{}
	_, err = client.Do(rebound) // want "owned resource from http.Do is not released on every return path"
	return err
}

func keepHeader(header http.Header) {
	_ = header
}

func headHeaderEscapes(url string) error {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return err
	}
	keepHeader(request.Header)
	client := &http.Client{}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	return err
}

func headClientCellCapturedByWorker(urls []string) error {
	client := &http.Client{}
	request, err := http.NewRequest("HEAD", urls[0], nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request)
	if err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, url := range urls {
			get, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return
			}
			response, err := client.Do(get)
			if err != nil {
				return
			}
			_ = response.Body.Close()
		}
	}()
	<-done
	return nil
}

func headClientCellConfiguredByWorker(urls []string) error {
	client := &http.Client{}
	request, err := http.NewRequest("HEAD", urls[0], nil)
	if err != nil {
		return err
	}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		client.Timeout = time.Second
	}()
	<-done
	return nil
}

func headClientCellReplaced(urls []string, other *http.Client) error {
	client := &http.Client{}
	request, err := http.NewRequest("HEAD", urls[0], nil)
	if err != nil {
		return err
	}
	if len(urls) > 1 {
		client = other
	}
	_, err = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = client.Do(request) // want "owned resource from http.Do is not released on every return path"
	}()
	<-done
	return nil
}
