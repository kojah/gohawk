package deferinloop

import (
	"net/http"
	"os"
)

func requestBodyConsumed(names []string, client *http.Client) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		request, err := http.NewRequest("POST", "https://example.invalid", file)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			continue
		}
		response.Body.Close()
	}
	return nil
}

func onlyBorrowedFile(names []string) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
		_, _ = file.Stat()
	}
	return nil
}

func requestNeverConsumed(names []string) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
		_, err = http.NewRequest("POST", "https://example.invalid", file)
		if err != nil {
			continue
		}
	}
	return nil
}
