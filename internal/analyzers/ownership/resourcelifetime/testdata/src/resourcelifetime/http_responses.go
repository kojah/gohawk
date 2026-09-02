package resourcelifetime

import (
	"bytes"
	"io"
	"net/http"
	"resourcedep"

	"github.com/stretchr/testify/assert"
)

func leakedResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	_ = response
	return nil
}

func closedResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func responseClosedByImportedDeferredCallback(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	resourcedep.CloseResponse(response)
	return nil
}

func responseConditionallyClosedByImportedDeferredCallback(client *http.Client, request *http.Request, enabled bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.MaybeCloseResponse(response, enabled)
	return nil
}

func responseBodyClosedByImportedHelper(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	resourcedep.CloseBody(response.Body)
	return nil
}

func responseBodyClosedByDeferredImportedHelper(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resourcedep.CloseBody(response.Body)
	return nil
}

func responseBodyConditionallyClosedByImportedHelper(client *http.Client, request *http.Request, enabled bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.MaybeCloseBody(response.Body, enabled)
	return nil
}

func importedHelperClosesSiblingBody(client *http.Client, first, second *http.Request) error {
	leaked, err := client.Do(first) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	other, err := client.Do(second)
	if err != nil {
		_ = leaked.Body.Close()
		return err
	}
	resourcedep.CloseBody(other.Body)
	return nil
}

func importedHelperSelectedOwnerMayDiffer(client *http.Client, request *http.Request, choose bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	other := &http.Response{Body: io.NopCloser(bytes.NewReader(nil))}
	selected := other
	if choose {
		selected = response
	}
	resourcedep.CloseBody(selected.Body)
	return nil
}

func reassignedResponseBodyDoesNotSettleImportedProjection(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	response.Body = io.NopCloser(bytes.NewReader(nil))
	resourcedep.CloseBody(response.Body)
	return nil
}

func opaqueResponseMutationDoesNotSettleImportedProjection(
	client *http.Client,
	request *http.Request,
	mutate func(*http.Response),
) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	mutate(response)
	resourcedep.CloseBody(response.Body)
	return nil
}

func opaqueBodyAddressMutationDoesNotSettleImportedProjection(
	client *http.Client,
	request *http.Request,
	mutate func(*io.ReadCloser),
) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	mutate(&response.Body)
	resourcedep.CloseBody(response.Body)
	return nil
}

func responseClosedAfterNoErrorGuard(t assert.TestingT, client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if !assert.NoError(t, err) {
		return err
	}
	defer response.Body.Close()
	return nil
}

func responseClosedInsideNoErrorGuard(t assert.TestingT, client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if assert.NoError(t, err) {
		_ = response.Body.Close()
	}
	return nil
}

func noErrorGuardChecksDifferentError(t assert.TestingT, client *http.Client, request *http.Request, other error) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if !assert.NoError(t, other) {
		return other
	}
	defer response.Body.Close()
	return err
}

func reversedNoErrorGuardLeaksResponse(t assert.TestingT, client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if assert.NoError(t, err) {
		return nil
	}
	defer response.Body.Close()
	return err
}

func responseBodyClosedThroughDeferredParameter(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func(body io.ReadCloser) {
		_ = body.Close()
	}(response.Body)
	return nil
}

func drainAndCloseResponseBody(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

func responseBodyClosedThroughDeferredStaticHelper(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer drainAndCloseResponseBody(response.Body)
	return nil
}

func closeOtherResponseBody(other, body io.ReadCloser) {
	_ = other.Close()
}

func deferredStaticHelperClosesSiblingResponse(client *http.Client, first, second *http.Request) error {
	leaked, err := client.Do(first) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	other, err := client.Do(second)
	if err != nil {
		_ = leaked.Body.Close()
		return err
	}
	defer closeOtherResponseBody(other.Body, leaked.Body)
	return nil
}

func partiallyCloseResponseBody(body io.ReadCloser, enabled bool) {
	if enabled {
		_ = body.Close()
	}
}

func deferredStaticHelperPartiallyClosesBody(client *http.Client, request *http.Request, enabled bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	defer partiallyCloseResponseBody(response.Body, enabled)
	return nil
}

func reassignAndCloseResponseBody(body, other io.ReadCloser) {
	body = other
	_ = body.Close()
}

func deferredStaticHelperReassignsBody(client *http.Client, first, second *http.Request) error {
	leaked, err := client.Do(first) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	other, err := client.Do(second)
	if err != nil {
		_ = leaked.Body.Close()
		return err
	}
	defer reassignAndCloseResponseBody(leaked.Body, other.Body)
	return nil
}

func indirectlyReassignAndCloseResponseBody(body, other io.ReadCloser) {
	slot := &body
	*slot = other
	_ = (*slot).Close()
}

func deferredStaticHelperIndirectlyReassignsBody(client *http.Client, request *http.Request) error {
	leaked, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	other := io.NopCloser(bytes.NewReader(nil))
	defer indirectlyReassignAndCloseResponseBody(leaked.Body, other)
	return nil
}

func chooseAndCloseResponseBody(body, other io.ReadCloser, choose bool) {
	chosen := other
	if choose {
		chosen = body
	}
	_ = chosen.Close()
}

func deferredStaticHelperPhiMayCloseDifferentBody(client *http.Client, request *http.Request, choose bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	other := io.NopCloser(bytes.NewReader(nil))
	defer chooseAndCloseResponseBody(response.Body, other, choose)
	return nil
}

func deferredStaticHelperSelectedOwnerMayDiffer(client *http.Client, request *http.Request, choose bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	other := &http.Response{Body: io.NopCloser(bytes.NewReader(nil))}
	selected := other
	if choose {
		selected = response
	}
	defer drainAndCloseResponseBody(selected.Body)
	return nil
}

func deferredStaticHelperOpaqueOwnerMutation(
	client *http.Client,
	request *http.Request,
	mutateOwner func(**http.Response),
) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	mutateOwner(&response)
	defer drainAndCloseResponseBody(response.Body)
	return nil
}

func deferredParameterClosesDifferentResponse(client *http.Client, first, second *http.Request) error {
	leaked, err := client.Do(first) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	closed, err := client.Do(second)
	if err != nil {
		_ = leaked.Body.Close()
		return err
	}
	defer func(body io.ReadCloser) {
		_ = body.Close()
	}(closed.Body)
	_ = leaked
	return nil
}

func deferredParameterClosesDifferentParameter(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	defer func(unrelated, body io.ReadCloser) {
		_ = unrelated.Close()
	}(io.NopCloser(bytes.NewReader(nil)), response.Body)
	return nil
}

func deferredParameterConditionallyCloses(client *http.Client, request *http.Request, enabled bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	defer func(body io.ReadCloser) {
		if enabled {
			_ = body.Close()
		}
	}(response.Body)
	return nil
}

func immediateParameterCloseDoesNotSettleReturn(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	func(body io.ReadCloser) {
		_ = body.Close()
	}(response.Body)
	return nil
}

func invokeCleanup(cleanup func() error) { _ = cleanup() }

func maybeInvokeCleanup(cleanup func() error, enabled bool) {
	if enabled {
		_ = cleanup()
	}
}

func observeCleanup(func() error) {}

func responseClosedByDeferredHelper(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer invokeCleanup(response.Body.Close)
	return nil
}

func conditionalDeferredHelperLeaksResponse(client *http.Client, request *http.Request, enabled bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	defer maybeInvokeCleanup(response.Body.Close, enabled)
	return nil
}

func nonDeferredObserverLeaksResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	observeCleanup(response.Body.Close)
	return nil
}

func deferredHelperClosesOnlyItsBoundResponse(client *http.Client, first, second *http.Request) error {
	closed, err := client.Do(first)
	if err != nil {
		return err
	}
	leaked, err := client.Do(second) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		_ = closed.Body.Close()
		return err
	}
	defer invokeCleanup(closed.Body.Close)
	_ = leaked
	return nil
}

func responseClosedByImmediateNestedDefer(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	func() {
		defer func() { _ = response.Body.Close() }()
	}()
	return nil
}

func responseConditionallyClosedByImmediateNestedDefer(client *http.Client, request *http.Request, closeBody bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	func() {
		if closeBody {
			defer func() { _ = response.Body.Close() }()
		}
	}()
	return nil
}

func responseCleanupClosureNotCalled(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	cleanup := func() { _ = response.Body.Close() }
	_ = cleanup
	return nil
}

func returnedResponseBody(client *http.Client, request *http.Request) (io.ReadCloser, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

func responseOwnedByWorker(client *http.Client, request *http.Request) (<-chan struct{}, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer response.Body.Close()
	}()
	return done, nil
}

func conditionallyReturnedResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}
