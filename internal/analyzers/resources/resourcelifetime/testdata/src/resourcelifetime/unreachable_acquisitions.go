package resourcelifetime

// An acquisition no feasible path reaches owes nothing. A helper that returns
// a nil response whenever its error is non-nil makes a retry guarded by
// res != nil after the error check unreachable, so the retried response is
// not a leak even though its success path never closes it. A retry guarded by
// anything the path has not settled stays reachable.
// https://github.com/gan-of-culture/get-sauce/blob/d726f56e7424bde4ff31e5329f37018343956103/request/request.go#L308-L318

import "net/http"

func FetchNilOnError(client *http.Client, url string) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	return response, nil
}

func retryAfterNilOnError(client *http.Client, url string) (http.Header, error) {
	res, err := FetchNilOnError(client, url) // want "owned resource from resourcelifetime.FetchNilOnError is not released"
	if err == nil {
		return res.Header, nil
	}
	if res != nil && res.StatusCode == 503 {
		res, err := FetchNilOnError(client, url)
		if err == nil {
			return res.Header, nil
		}
	}
	return nil, err
}

func retryWhenAsked(client *http.Client, url string, retry bool) (http.Header, error) {
	res, err := FetchNilOnError(client, url) // want "owned resource from resourcelifetime.FetchNilOnError is not released"
	if err == nil {
		return res.Header, nil
	}
	if retry {
		res, err := FetchNilOnError(client, url) // want "owned resource from resourcelifetime.FetchNilOnError is not released"
		if err == nil {
			return res.Header, nil
		}
	}
	return nil, err
}
