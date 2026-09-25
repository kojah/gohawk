package resourcelifetime

// A guard that repeats a test the path already settled cannot take its other
// arm. Two reads of the same field of a response a call returned read the
// same slot, so once the first check proved the status 200, a second check of
// that status cannot send this path to the early return that leaves the new
// response open. The repeat stays uncertain rather than infeasible, since a
// hidden store could change the field, so it hides the diagnostic without
// proving the close. A second check of a value the path has not settled, such
// as the new response's own status, can still fail and leak it. The first
// response's own non-200 return leaks it in both forms.
// https://github.com/OperantAI/woodpecker/blob/2062edfefd2b1d660238dc9a2e6f75456d0752ae/internal/experiments/experiments_ai_data_leakage.go#L86-L130

import "net/http"

func repeatedFirstStatus(client *http.Client, first, second *http.Request) error {
	firstResponse, err := client.Do(first) // want "owned resource from http.Do is not released"
	if err != nil || firstResponse.StatusCode != 200 {
		return err
	}
	defer firstResponse.Body.Close()
	secondResponse, err := client.Do(second)
	if err != nil || firstResponse.StatusCode != 200 {
		return err
	}
	defer secondResponse.Body.Close()
	return nil
}

func checkedSecondStatus(client *http.Client, first, second *http.Request) error {
	firstResponse, err := client.Do(first) // want "owned resource from http.Do is not released"
	if err != nil || firstResponse.StatusCode != 200 {
		return err
	}
	defer firstResponse.Body.Close()
	secondResponse, err := client.Do(second) // want "owned resource from http.Do is not released"
	if err != nil || secondResponse.StatusCode != 200 {
		return err
	}
	defer secondResponse.Body.Close()
	return nil
}

// Inside a loop the repeated check still refers to this iteration's first
// response: the guard holds until the next iteration calls Do again.
func repeatedFirstStatusInLoop(client *http.Client, pairs [][2]*http.Request) error {
	for _, pair := range pairs {
		firstResponse, err := client.Do(pair[0]) // want "owned resource from http.Do is not released"
		if err != nil || firstResponse.StatusCode != 200 {
			return err
		}
		secondResponse, err := client.Do(pair[1])
		if err != nil || firstResponse.StatusCode != 200 {
			firstResponse.Body.Close()
			return err
		}
		secondResponse.Body.Close()
		firstResponse.Body.Close()
	}
	return nil
}
