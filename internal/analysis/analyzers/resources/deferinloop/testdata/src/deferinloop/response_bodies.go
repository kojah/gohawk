package deferinloop

import (
	"io"
	"net/http"
)

// A response body is the resource a client call hands back: closing the
// Body field of the *http.Response acquired in this iteration is its cleanup.
// Only the standard response type's own Body field qualifies.

func bodyDeferredPerRequest(client *http.Client, urls []string) ([][]byte, error) {
	var out [][]byte
	for _, url := range urls {
		response, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
		data, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		out = append(out, data)
	}
	return out, nil
}

// Every path after the defer leaves the function, so Go runs the cleanup
// before another request can be made.
func firstReachableBody(client *http.Client, urls []string) ([]byte, error) {
	for _, url := range urls {
		response, err := client.Get(url)
		if err != nil {
			continue
		}
		defer response.Body.Close()
		return io.ReadAll(response.Body)
	}
	return nil, nil
}

// The body is closed within the iteration; the defer is not what releases it.
func bodyClosedEachIteration(client *http.Client, urls []string) error {
	for _, url := range urls {
		response, err := client.Get(url)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		_, err = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// The responses were acquired before the loop; each iteration defers a
// cleanup it did not acquire, which is a single function-wide lifetime.
func bodiesAcquiredElsewhere(responses []*http.Response) {
	for _, response := range responses {
		defer response.Body.Close()
	}
}

// The response escapes into a collection the caller receives, so its body's
// lifetime is not the iteration's.
func responsesKept(client *http.Client, urls []string) []*http.Response {
	var kept []*http.Response
	for _, url := range urls {
		response, err := client.Get(url)
		if err != nil {
			continue
		}
		kept = append(kept, response)
		defer response.Body.Close()
	}
	return kept
}

// The Body field now holds a different reader than the one the response
// arrived with; which value the defer closes is not the acquired body.
func bodyReplacedBeforeDefer(client *http.Client, urls []string, wrap func(io.ReadCloser) io.ReadCloser) {
	for _, url := range urls {
		response, err := client.Get(url)
		if err != nil {
			continue
		}
		response.Body = wrap(response.Body)
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
	}
}

// A project type with a Body field is not a response: the name alone carries
// no lifecycle contract.
type envelope struct {
	Body io.ReadCloser
}

func fetchEnvelope(url string) (*envelope, error) {
	return &envelope{Body: io.NopCloser(nil)}, nil
}

func envelopeBodies(urls []string) {
	for _, url := range urls {
		message, err := fetchEnvelope(url)
		if err != nil {
			continue
		}
		defer message.Body.Close()
		_, _ = io.Copy(io.Discard, message.Body)
	}
}
