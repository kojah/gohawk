package resourcelifetime

import (
	"errors"
	"net"
	"net/http"
	"os"
)

// Error predicates establish failed acquisition only for the exact error from
// that call. A negative match, unrelated error, or joined error cannot do so.
func closeResponseAfterAs(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return err
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func closeResponseAfterNestedAs(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) && dnsError.IsNotFound {
		return err
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func unrelatedAsDoesNotProveFailure(client *http.Client, request *http.Request, other error) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	var dnsError *net.DNSError
	if errors.As(other, &dnsError) {
		return other
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func joinedAsDoesNotProveFailure(client *http.Client, request *http.Request, other error) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	var dnsError *net.DNSError
	if errors.As(errors.Join(err, other), &dnsError) {
		return other
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func negativeAsDoesNotProveFailure(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	var dnsError *net.DNSError
	if !errors.As(err, &dnsError) {
		return err
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func closeFileAfterSentinelEquality(path string) error {
	file, err := os.Open(path)
	if err == os.ErrNotExist {
		return err
	}
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func closeFileAfterReversedSentinelEquality(path string) error {
	file, err := os.Open(path)
	if os.ErrExist == err {
		return err
	}
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func arbitrarySentinelCanBeNil(path string, sentinel error) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err == sentinel {
		return err
	}
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func unrelatedSentinelEquality(path string, other error) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if other == os.ErrNotExist {
		return other
	}
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func sentinelInequalityCanSucceed(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != os.ErrNotExist {
		return err
	}
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}
